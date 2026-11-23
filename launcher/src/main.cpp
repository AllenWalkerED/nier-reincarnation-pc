// NieR.exe -- the launcher for NieR Re[in]carnation on Windows.
//
// A window with a WebView2 page (launcher\ui\index.html, served from disk as
// https://nier.launcher/). The page keeps the settings (settings.json next to
// this program) and asks for a game; this program then:
//   1. writes the mods (save\mods.json), opens the shops and the events chosen
//      in Settings › Events (save\active-content.json) in the master data when
//      they changed (tools\patch_content.py), and starts the server (Lunar Tear with Bunker's changes)
//      (nier-server: auth, CDN, gRPC) hidden, bound to 127.0.0.1, in a job
//      object that ends it with this program;
//   2. waits until the three ports answer;
//   3. starts the game on the win.apk runtime with the settings' environment
//      (display, graphics, controls), hides itself, and waits for it to end;
//   4. stops the servers and shows itself again.
//
// Messages from the page are strings: a command on the first line, then its
// lines. "load"; "save" + the settings JSON; "play" + "input:<json>" + "mods:<json>" + "pref:NAME=VALUE"... +
// "env:NAME=VALUE"...; "logs"; "export-logs"; "quit". To the page: "settings:<json>",
// "status:<text>", "error:<text>", "exited:<code>", "exporting:", "exported:<path>",
// "export-error:<text>".

#include <windows.h>
#include <dwmapi.h>
#include <dxgi.h>
#include <shellapi.h>
#include <tlhelp32.h>
#include <winsock2.h>
#include <ws2tcpip.h>

#include <atomic>
#include <cstdio>
#include <functional>
#include <sstream>
#include <string>
#include <thread>
#include <vector>

#include <knownfolders.h>
#include <shlobj.h>
#include <shobjidl.h>

#include "WebView2.h"

namespace {

// ---- COM helpers (as win.apk's webview2_host.h) -----------------------------

template <typename T>
class ComPtr {
public:
    ComPtr() = default;
    ComPtr(const ComPtr&) = delete;
    ComPtr& operator=(const ComPtr&) = delete;
    ~ComPtr() { reset(); }
    void reset() {
        if (p_) p_->Release();
        p_ = nullptr;
    }
    T** put() {
        reset();
        return &p_;
    }
    void retain(T* p) {
        if (p) p->AddRef();
        reset();
        p_ = p;
    }
    T* get() const { return p_; }
    T* operator->() const { return p_; }
    explicit operator bool() const { return p_ != nullptr; }

private:
    T* p_ = nullptr;
};

template <typename I, typename... A>
class Handler final : public I {
public:
    using Fn = std::function<HRESULT(A...)>;
    Handler(const IID& iid, Fn fn) : iid_(iid), fn_(std::move(fn)) {}
    HRESULT STDMETHODCALLTYPE QueryInterface(REFIID riid, void** out) override {
        if (!out) return E_POINTER;
        if (IsEqualIID(riid, IID_IUnknown) || IsEqualIID(riid, iid_)) {
            *out = static_cast<I*>(this);
            AddRef();
            return S_OK;
        }
        *out = nullptr;
        return E_NOINTERFACE;
    }
    ULONG STDMETHODCALLTYPE AddRef() override { return ++refs_; }
    ULONG STDMETHODCALLTYPE Release() override {
        const ULONG r = --refs_;
        if (r == 0) delete this;
        return r;
    }
    HRESULT STDMETHODCALLTYPE Invoke(A... a) override { return fn_ ? fn_(a...) : S_OK; }

private:
    const IID iid_;
    Fn fn_;
    std::atomic<ULONG> refs_{1};
};
template <typename M>
struct HandlerOf;
template <typename I, typename... A>
struct HandlerOf<HRESULT (I::*)(A...)> {
    using type = Handler<I, A...>;
};
template <typename I, typename F>
I* make_handler(const IID& iid, F&& fn) {
    using H = typename HandlerOf<decltype(&I::Invoke)>::type;
    return new H(iid, std::forward<F>(fn));
}

// ---- paths and text -------------------------------------------------------------

std::wstring g_root;  // the folder of NieR.exe: pc-port

std::wstring path(const std::wstring& rel) { return g_root + L"\\" + rel; }

std::wstring widen(const std::string& s) {
    if (s.empty()) return {};
    const int n = MultiByteToWideChar(CP_UTF8, 0, s.data(), static_cast<int>(s.size()), nullptr, 0);
    std::wstring w(static_cast<size_t>(n), L'\0');
    MultiByteToWideChar(CP_UTF8, 0, s.data(), static_cast<int>(s.size()), &w[0], n);
    return w;
}
std::string narrow(const std::wstring& w) {
    if (w.empty()) return {};
    const int n = WideCharToMultiByte(CP_UTF8, 0, w.data(), static_cast<int>(w.size()), nullptr, 0, nullptr, nullptr);
    std::string s(static_cast<size_t>(n), '\0');
    WideCharToMultiByte(CP_UTF8, 0, w.data(), static_cast<int>(w.size()), &s[0], n, nullptr, nullptr);
    return s;
}

std::string read_file(const std::wstring& p) {
    std::string out;
    FILE* f = _wfopen(p.c_str(), L"rb");
    if (!f) return out;
    char buf[65536];
    for (size_t n; (n = fread(buf, 1, sizeof buf, f)) > 0;) out.append(buf, n);
    fclose(f);
    return out;
}
bool write_file(const std::wstring& p, const std::string& data) {
    const std::wstring tmp = p + L".tmp";
    FILE* f = _wfopen(tmp.c_str(), L"wb");
    if (!f) return false;
    const bool ok = fwrite(data.data(), 1, data.size(), f) == data.size();
    fclose(f);
    return ok && MoveFileExW(tmp.c_str(), p.c_str(), MOVEFILE_REPLACE_EXISTING) != 0;
}

// ---- the window and the page ---------------------------------------------------

constexpr UINT kMsgToPage = WM_APP + 1;  // lParam: std::wstring* to post to the page
constexpr UINT kMsgShow = WM_APP + 2;    // wParam: show (1) or hide (0)
HWND g_hwnd = nullptr;
ComPtr<ICoreWebView2Controller> g_controller;
ComPtr<ICoreWebView2> g_webview;
std::atomic<bool> g_busy{false};

void to_page(const std::wstring& text) {
    if (g_hwnd) PostMessageW(g_hwnd, kMsgToPage, 0, reinterpret_cast<LPARAM>(new std::wstring(text)));
}
void status(const std::string& text) { to_page(L"status:" + widen(text)); }

// ---- processes -------------------------------------------------------------------

HANDLE g_job = nullptr;  // servers and game: they end with this program

HANDLE make_job() {
    HANDLE job = CreateJobObjectW(nullptr, nullptr);
    JOBOBJECT_EXTENDED_LIMIT_INFORMATION info{};
    info.BasicLimitInformation.LimitFlags = JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE;
    SetInformationJobObject(job, JobObjectExtendedLimitInformation, &info, sizeof(info));
    return job;
}

// Runs exe with args, its output appended to log; env is a full environment
// block (nullptr: ours). Hidden unless `window`: the game's own window takes
// the show state given here the first time it is shown, so the game is given
// SW_SHOWNORMAL (with SW_HIDE its window stayed hidden in window mode). The
// process handle, or nullptr.
HANDLE spawn(const std::wstring& exe, const std::wstring& args, const std::wstring& cwd, const std::wstring& log,
             const std::wstring* env, HANDLE job, bool window = false) {
    SECURITY_ATTRIBUTES sa{sizeof(sa), nullptr, TRUE};
    HANDLE out = CreateFileW(log.c_str(), FILE_APPEND_DATA, FILE_SHARE_READ | FILE_SHARE_WRITE, &sa, OPEN_ALWAYS,
                             FILE_ATTRIBUTE_NORMAL, nullptr);
    STARTUPINFOW si{};
    si.cb = sizeof(si);
    si.dwFlags = STARTF_USESTDHANDLES | STARTF_USESHOWWINDOW;
    si.wShowWindow = window ? SW_SHOWNORMAL : SW_HIDE;
    si.hStdInput = GetStdHandle(STD_INPUT_HANDLE);
    si.hStdOutput = out;
    si.hStdError = out;
    std::wstring cmd = L"\"" + exe + L"\" " + args;
    PROCESS_INFORMATION pi{};
    const BOOL ok = CreateProcessW(exe.c_str(), &cmd[0], nullptr, nullptr, TRUE,
                                   CREATE_NO_WINDOW | CREATE_SUSPENDED | (env ? CREATE_UNICODE_ENVIRONMENT : 0),
                                   env ? const_cast<wchar_t*>(env->c_str()) : nullptr, cwd.c_str(), &si, &pi);
    if (out != INVALID_HANDLE_VALUE) CloseHandle(out);
    if (!ok) return nullptr;
    if (job) AssignProcessToJobObject(job, pi.hProcess);
    ResumeThread(pi.hThread);
    CloseHandle(pi.hThread);
    return pi.hProcess;
}

// Ends running copies of a program by its full path (servers a previous
// run-server.ps1 or a crashed launcher left behind hold the ports).
void kill_by_path(const std::wstring& exe) {
    HANDLE snap = CreateToolhelp32Snapshot(TH32CS_SNAPPROCESS, 0);
    PROCESSENTRY32W pe{};
    pe.dwSize = sizeof(pe);
    for (BOOL ok = Process32FirstW(snap, &pe); ok; ok = Process32NextW(snap, &pe)) {
        HANDLE h = OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION | PROCESS_TERMINATE | SYNCHRONIZE, FALSE,
                               pe.th32ProcessID);
        if (!h) continue;
        wchar_t buf[MAX_PATH * 2];
        DWORD n = MAX_PATH * 2;
        if (QueryFullProcessImageNameW(h, 0, buf, &n) && _wcsicmp(buf, exe.c_str()) == 0) {
            TerminateProcess(h, 1);
            WaitForSingleObject(h, 3000);
        }
        CloseHandle(h);
    }
    CloseHandle(snap);
}

bool port_open(int port) {
    SOCKET s = socket(AF_INET, SOCK_STREAM, IPPROTO_TCP);
    if (s == INVALID_SOCKET) return false;
    sockaddr_in a{};
    a.sin_family = AF_INET;
    a.sin_port = htons(static_cast<u_short>(port));
    a.sin_addr.s_addr = htonl(INADDR_LOOPBACK);
    const bool ok = connect(s, reinterpret_cast<sockaddr*>(&a), sizeof(a)) == 0;
    closesocket(s);
    return ok;
}

// Our environment with the settings' variables set (a Unicode block).
std::wstring environment_with(const std::vector<std::pair<std::wstring, std::wstring>>& vars) {
    std::vector<std::wstring> entries;
    wchar_t* block = GetEnvironmentStringsW();
    for (wchar_t* p = block; *p; p += wcslen(p) + 1) {
        std::wstring e(p);
        bool replaced = false;
        for (const auto& v : vars) {
            if (_wcsnicmp(e.c_str(), (v.first + L"=").c_str(), v.first.size() + 1) == 0) replaced = true;
        }
        if (!replaced) entries.push_back(e);
    }
    FreeEnvironmentStringsW(block);
    for (const auto& v : vars) entries.push_back(v.first + L"=" + v.second);
    std::wstring out;
    for (const auto& e : entries) out += e + L'\0';
    out += L'\0';
    return out;
}

// ---- a game -----------------------------------------------------------------------

struct Launch {
    std::string input_json;
    std::string mods_json;  // save\mods.json, as the page builds it
    std::vector<std::pair<std::wstring, std::wstring>> env;
    std::vector<std::pair<std::string, std::string>> prefs;  // the game's PlayerPrefs strings to set ("" removes)
};

// The game's PlayerPrefs (an Android shared_prefs XML in the runtime data):
// <string name="NAME">VALUE</string> set, replaced, or removed when VALUE is
// empty. The values come from the page already in the game's form (its DES
// encryption, then base64 and URL encoding: app.js).
void set_game_prefs(const std::wstring& runtime_data, const std::vector<std::pair<std::string, std::string>>& prefs) {
    if (prefs.empty()) return;
    const std::wstring file = runtime_data +
        L"\\android\\data\\data\\NieR-loopback\\shared_prefs\\com.square_enix.android_googleplay.nierspww.v2.playerprefs.xml";
    std::string xml = read_file(file);
    if (xml.find("</map>") == std::string::npos) return;  // the game has not made it yet: its first run decides
    for (const auto& kv : prefs) {
        const std::string open = "<string name=\"" + kv.first + "\">";
        size_t at = xml.find(open);
        if (at != std::string::npos) {
            size_t end = xml.find("</string>", at);
            if (end == std::string::npos) continue;
            end += 9;
            while (end < xml.size() && (xml[end] == '\r' || xml[end] == '\n')) ++end;
            size_t start = xml.rfind('\n', at);
            start = start == std::string::npos ? at : start + 1;
            xml.erase(start, end - start);
        }
        if (!kv.second.empty()) {
            const size_t map_end = xml.rfind("</map>");
            xml.insert(map_end, "    " + open + kv.second + "</string>\n");
        }
    }
    write_file(file, xml);
}

void run_game(Launch l) {
    const std::wstring bin = path(L"server\\bin"), data = path(L"data"), save = path(L"save"), logs = path(L"logs");
    CreateDirectoryW(save.c_str(), nullptr);
    CreateDirectoryW(logs.c_str(), nullptr);

    status("Stopping old servers...");
    for (const wchar_t* name : {L"nier-server.exe", L"auth-server.exe", L"octo-cdn.exe", L"lunar-tear.exe"})
        kill_by_path(bin + L"\\" + name);

    // The accounts' key stays the same between runs (save\auth.secret;
    // nier-server copies it to auth.key, the name the server reads).
    std::string secret = read_file(save + L"\\auth.secret");
    while (!secret.empty() && (secret.back() == '\n' || secret.back() == '\r' || secret.back() == ' ')) secret.pop_back();
    if (secret.empty()) {
        static const char hex[] = "0123456789abcdef";
        for (int i = 0; i < 64; ++i) secret += hex[(GetTickCount64() * 2654435761ull + i * 40503ull + rand()) % 16];
        write_file(save + L"\\auth.secret", secret);
    }

    // The mods (the server reads save\mods.json at each call that uses one).
    if (!l.mods_json.empty()) write_file(save + L"\\mods.json", l.mods_json);

    // The shops and events opened again in the master data, once
    // (tools\patch_content.py; it does nothing when that is done already).
    // Without it the game still runs, with the original shop.
    status("Preparing the game data...");
    {
        const std::wstring py = path(L"tools\\python\\python.exe");
        std::vector<std::pair<std::wstring, std::wstring>> extra{{L"PYTHONUTF8", L"1"}, {L"PYTHONIOENCODING", L"utf-8"}};
        const std::wstring env = environment_with(extra);
        // Only the events chosen in Settings › Events (save\active-content.json; none until chosen).
        const std::wstring args = L"-u \"" + path(L"tools\\patch_content.py") + L"\" --data \"" + data +
                                  L"\" --selection \"" + save + L"\\active-content.json\"";
        if (HANDLE p = spawn(py, args, g_root,
                             logs + L"\\content-patch.log", &env, g_job)) {
            WaitForSingleObject(p, 300000);
            CloseHandle(p);
        }
    }

    // The server (Lunar Tear): the game (8003), the assets (8080) and the accounts
    // (3000) in one process; it snapshots and migrates the save itself.
    status("Starting the server...");
    HANDLE servers[1] = {
        spawn(bin + L"\\nier-server.exe", L"--save \"" + save + L"\" --data \"" + data + L"\"", data,
              logs + L"\\server.log", nullptr, g_job),
    };
    bool ready = false;
    for (int i = 0; i < 600 && !ready; ++i) {
        ready = port_open(3000) && port_open(8080) && port_open(8003);
        if (!ready) Sleep(100);
    }
    if (!ready) {
        to_page(L"error:The server did not start. See logs\\server.log.");
        for (HANDLE h : servers) {
            if (h) {
                TerminateProcess(h, 1);
                CloseHandle(h);
            }
        }
        g_busy = false;
        return;
    }

    // The controls: a file the runtime reads (WINAPK_INPUT_MAP).
    if (!l.input_json.empty()) {
        write_file(path(L"input-map.json"), l.input_json);
        l.env.emplace_back(L"WINAPK_INPUT_MAP", path(L"input-map.json"));
    }
    // The game window's cursor: the port's cursors\nier.cur (an original
    // design, no game art), whatever an older page asked for (it named
    // cursors\normal.ani). A player who wants another one puts it in as
    // cursors\custom.cur or cursors\custom.ani. The port's triangle is shown
    // half again as big as the system's cursor (WINAPK_CURSOR_SCALE).
    bool big_cursor = false;
    for (auto& v : l.env) {
        if (v.first != L"WINAPK_CURSOR") continue;
        v.second = path(L"cursors\\nier.cur");
        big_cursor = true;
        for (const wchar_t* name : {L"cursors\\custom.ani", L"cursors\\custom.cur"}) {
            if (GetFileAttributesW(path(name).c_str()) != INVALID_FILE_ATTRIBUTES) {
                v.second = path(name);
                big_cursor = false;
                break;
            }
        }
    }
    if (big_cursor) l.env.emplace_back(L"WINAPK_CURSOR_SCALE", L"1.5");
    std::wstring runtime = path(L"runtime\\bin\\winapk-runtime.exe"), runtime_data = path(L"runtime-data");
    for (const auto& v : l.env) {
        if (v.first == L"NIER_RUNTIME") runtime = v.second;
        if (v.first == L"NIER_RUNTIME_DATA") runtime_data = v.second;
    }
    set_game_prefs(runtime_data, l.prefs);
    const std::wstring env = environment_with(l.env);
    status("Starting the game...");
    HANDLE game = spawn(runtime,
                        L"--data \"" + runtime_data + L"\" --apk \"" + path(L"client\\NieR-loopback.apk") + L"\"",
                        g_root, logs + L"\\game.log", &env, g_job, true);
    if (!game) {
        to_page(L"error:The game did not start (" + runtime + L").");
    } else {
        Sleep(1500);
        PostMessageW(g_hwnd, kMsgShow, 0, 0);
        WaitForSingleObject(game, INFINITE);
        DWORD code = 0;
        GetExitCodeProcess(game, &code);
        CloseHandle(game);
        // The runtime's helper processes go with it.
        for (const wchar_t* name : {L"winapk-runtime.exe"}) kill_by_path(runtime.substr(0, runtime.find_last_of(L'\\') + 1) + name);
        to_page(L"exited:" + std::to_wstring(code));
    }
    for (HANDLE h : servers) {
        if (h) {
            TerminateProcess(h, 0);
            CloseHandle(h);
        }
    }
    PostMessageW(g_hwnd, kMsgShow, 1, 0);
    g_busy = false;
}

// ---- first-run setup ---------------------------------------------------------------
// The port ships without the game: tools/setup_game.py prepares it from the
// player's files (setup_game.py says what it makes). Ready when what it made
// is there: the patched game, the assets, and the game installed in the
// runtime's data folder (runtime-data, or the folder Settings › System ›
// Runtime data names). setup.json, its record, is not needed: an install
// prepared by hand, or one whose record was lost, still plays.

bool exists(const std::wstring& p) { return GetFileAttributesW(p.c_str()) != INVALID_FILE_ATTRIBUTES; }

// settings.json's paths.runtimeData ("" when unset): the one string read
// here, so a plain search, with JSON's \\ and \" undone.
std::wstring settings_runtime_data() {
    const std::string json = read_file(path(L"settings.json"));
    const size_t key = json.find("\"runtimeData\"");
    if (key == std::string::npos) return {};
    size_t at = json.find(':', key);
    if (at == std::string::npos) return {};
    at = json.find('"', at);
    if (at == std::string::npos) return {};
    std::string value;
    for (size_t i = at + 1; i < json.size() && json[i] != '"'; ++i) {
        if (json[i] == '\\' && i + 1 < json.size()) ++i;
        value += json[i];
    }
    return widen(value);
}

bool setup_ready() {
    if (!exists(path(L"client\\NieR-loopback.apk")) || !exists(path(L"data\\assets\\revisions\\0\\list.bin"))) return false;
    if (exists(path(L"runtime-data\\android"))) return true;
    const std::wstring custom = settings_runtime_data();
    return !custom.empty() && exists(custom + L"\\android");
}

// A file (or, for kind "assets-folder", a folder) the player picks.
std::wstring pick(const std::wstring& kind) {
    std::wstring chosen;
    IFileOpenDialog* dlg = nullptr;
    if (FAILED(CoCreateInstance(CLSID_FileOpenDialog, nullptr, CLSCTX_INPROC_SERVER, IID_PPV_ARGS(&dlg)))) return chosen;
    DWORD opts = 0;
    dlg->GetOptions(&opts);
    opts |= FOS_FORCEFILESYSTEM | FOS_PATHMUSTEXIST;
    if (kind == L"assets-folder") {
        opts |= FOS_PICKFOLDERS;
        dlg->SetTitle(L"The folder that holds revisions\\0");
    } else if (kind == L"apk") {
        static const COMDLG_FILTERSPEC f[] = {{L"Android game (.apk)", L"*.apk"}};
        dlg->SetFileTypes(1, f);
        dlg->SetTitle(L"NieR Re[in]carnation 3.7.1, arm64 APK");
    } else if (kind == L"assets") {
        static const COMDLG_FILTERSPEC f[] = {{L"Asset dump (.7z, .zip)", L"*.7z;*.zip"}};
        dlg->SetFileTypes(1, f);
        dlg->SetTitle(L"resource_dump_android.7z");
    } else if (kind == L"phone-save") {
        static const COMDLG_FILTERSPEC f[] = {{L"Save backup (.zip, game.db)", L"*.zip;*.db"}};
        dlg->SetFileTypes(1, f);
        dlg->SetTitle(L"A save backup from the phone (Bunker's Export save backup)");
    } else if (kind == L"master") {
        static const COMDLG_FILTERSPEC f[] = {{L"Master data (.bin.e)", L"*.bin.e"}, {L"All files", L"*.*"}};
        dlg->SetFileTypes(2, f);
        dlg->SetTitle(L"20240404193219.bin.e");
    }
    dlg->SetOptions(opts);
    if (SUCCEEDED(dlg->Show(g_hwnd))) {
        IShellItem* item = nullptr;
        if (SUCCEEDED(dlg->GetResult(&item))) {
            PWSTR p = nullptr;
            if (SUCCEEDED(item->GetDisplayName(SIGDN_FILESYSPATH, &p))) {
                chosen = p;
                CoTaskMemFree(p);
            }
            item->Release();
        }
    }
    dlg->Release();
    return chosen;
}

// setup_game.py with the bundled Python, its lines passed to the page as
// "setup-line:<line>" and its end as "setup-exit:<code>".
void run_setup(std::wstring apk, std::wstring assets, std::wstring master) {
    SECURITY_ATTRIBUTES sa{sizeof(sa), nullptr, TRUE};
    HANDLE read = nullptr, write = nullptr;
    if (!CreatePipe(&read, &write, &sa, 0)) {
        to_page(L"setup-exit:-1");
        g_busy = false;
        return;
    }
    SetHandleInformation(read, HANDLE_FLAG_INHERIT, 0);
    const std::wstring py = path(L"tools\\python\\python.exe");
    std::wstring cmd = L"\"" + py + L"\" -u \"" + path(L"tools\\setup_game.py") + L"\" --root \"" + g_root +
                       L"\" --apk \"" + apk + L"\" --assets \"" + assets + L"\" --master \"" + master + L"\"";
    STARTUPINFOW si{};
    si.cb = sizeof(si);
    si.dwFlags = STARTF_USESTDHANDLES | STARTF_USESHOWWINDOW;
    si.wShowWindow = SW_HIDE;
    si.hStdOutput = write;
    si.hStdError = write;
    PROCESS_INFORMATION pi{};
    // PYTHONUTF8: paths and messages in UTF-8 whatever the system code page.
    std::vector<std::pair<std::wstring, std::wstring>> extra{{L"PYTHONUTF8", L"1"}, {L"PYTHONIOENCODING", L"utf-8"}};
    const std::wstring env = environment_with(extra);
    const BOOL ok = CreateProcessW(py.c_str(), &cmd[0], nullptr, nullptr, TRUE, CREATE_NO_WINDOW | CREATE_UNICODE_ENVIRONMENT,
                                   const_cast<wchar_t*>(env.c_str()), g_root.c_str(), &si, &pi);
    CloseHandle(write);
    if (!ok) {
        CloseHandle(read);
        to_page(L"setup-line:error:The bundled Python was not found (tools\\python).");
        to_page(L"setup-exit:-1");
        g_busy = false;
        return;
    }
    if (g_job) AssignProcessToJobObject(g_job, pi.hProcess);
    CloseHandle(pi.hThread);
    std::string line;
    char buf[4096];
    DWORD n = 0;
    while (ReadFile(read, buf, sizeof buf, &n, nullptr) && n > 0) {
        for (DWORD i = 0; i < n; ++i) {
            if (buf[i] == '\n') {
                if (!line.empty() && line.back() == '\r') line.pop_back();
                to_page(L"setup-line:" + widen(line));
                line.clear();
            } else {
                line += buf[i];
            }
        }
    }
    CloseHandle(read);
    WaitForSingleObject(pi.hProcess, INFINITE);
    DWORD code = 1;
    GetExitCodeProcess(pi.hProcess, &code);
    CloseHandle(pi.hProcess);
    to_page(L"setup-exit:" + std::to_wstring(code));
    g_busy = false;
}

// ---- exporting the logs ------------------------------------------------------------
// One zip for a bug report: the logs (servers, game, the runtime's own), the
// settings and setup record, and system.txt (Windows, processor, memory, and
// each GPU with its driver). The Windows user and computer names in them are
// replaced, as they are in every path. Windows' own tar.exe (Windows 10 1803+)
// makes the zip.

std::atomic<bool> g_exporting{false};

std::wstring date_stamp() {
    SYSTEMTIME t;
    GetLocalTime(&t);
    wchar_t buf[32];
    swprintf(buf, 32, L"%04u%02u%02u-%02u%02u%02u", t.wYear, t.wMonth, t.wDay, t.wHour, t.wMinute, t.wSecond);
    return buf;
}

// Where to save the zip (the Desktop by default), or "" when cancelled.
std::wstring pick_save_path(const wchar_t* title, const std::wstring& file);
std::wstring pick_export_path() { return pick_save_path(L"Save the logs for a bug report", L"NieR-logs-" + date_stamp() + L".zip"); }

// A Save dialog for a .zip, on the desktop by default.
std::wstring pick_save_path(const wchar_t* title, const std::wstring& file) {
    std::wstring chosen;
    IFileSaveDialog* dlg = nullptr;
    if (FAILED(CoCreateInstance(CLSID_FileSaveDialog, nullptr, CLSCTX_INPROC_SERVER, IID_PPV_ARGS(&dlg)))) return chosen;
    static const COMDLG_FILTERSPEC f[] = {{L"Zip archive (.zip)", L"*.zip"}};
    dlg->SetFileTypes(1, f);
    dlg->SetDefaultExtension(L"zip");
    dlg->SetTitle(title);
    dlg->SetFileName(file.c_str());
    IShellItem* desktop = nullptr;
    if (SUCCEEDED(SHGetKnownFolderItem(FOLDERID_Desktop, KF_FLAG_DEFAULT, nullptr, IID_PPV_ARGS(&desktop)))) {
        dlg->SetDefaultFolder(desktop);
        desktop->Release();
    }
    DWORD opts = 0;
    dlg->GetOptions(&opts);
    dlg->SetOptions(opts | FOS_FORCEFILESYSTEM | FOS_OVERWRITEPROMPT);
    if (SUCCEEDED(dlg->Show(g_hwnd))) {
        IShellItem* item = nullptr;
        if (SUCCEEDED(dlg->GetResult(&item))) {
            PWSTR p = nullptr;
            if (SUCCEEDED(item->GetDisplayName(SIGDN_FILESYSPATH, &p))) {
                chosen = p;
                CoTaskMemFree(p);
            }
            item->Release();
        }
    }
    dlg->Release();
    return chosen;
}

// Every occurrence of `name` (ASCII letters in any case) replaced by `with`.
void replace_name(std::string* text, const std::string& name, const std::string& with) {
    if (name.size() < 3) return;  // too short to tell from ordinary words
    auto lower = [](char c) { return c >= 'A' && c <= 'Z' ? static_cast<char>(c + 32) : c; };
    std::string out;
    out.reserve(text->size());
    for (size_t i = 0; i < text->size();) {
        size_t k = 0;
        while (k < name.size() && i + k < text->size() && lower((*text)[i + k]) == lower(name[k])) ++k;
        if (k == name.size()) {
            out += with;
            i += k;
        } else {
            out += (*text)[i++];
        }
    }
    text->swap(out);
}

std::string scrubbed(std::string text) {
    wchar_t buf[256];
    DWORD n = 256;
    if (GetUserNameW(buf, &n)) replace_name(&text, narrow(buf), "USER");
    n = 256;
    if (GetComputerNameW(buf, &n)) replace_name(&text, narrow(buf), "PC");
    return text;
}

// A log's last 16 MB (the game's log grows with every session), from a line start.
std::string log_tail(const std::wstring& file) {
    constexpr size_t kMax = 16u << 20;
    std::string text = read_file(file);
    if (text.size() <= kMax) return text;
    size_t from = text.find('\n', text.size() - kMax);
    from = from == std::string::npos ? text.size() - kMax : from + 1;
    return "[export] the first " + std::to_string(from) + " bytes of this log were left out\n" + text.substr(from);
}

std::string reg_string(HKEY root, const wchar_t* key, const wchar_t* value) {
    wchar_t buf[512];
    DWORD size = sizeof buf;
    if (RegGetValueW(root, key, value, RRF_RT_REG_SZ, nullptr, buf, &size) != ERROR_SUCCESS) return {};
    return narrow(buf);
}

std::string system_report() {
    std::ostringstream r;
    r << "NieR Re[in]carnation PC port: logs exported " << narrow(date_stamp()) << "\n\n";

    using RtlGetVersion = LONG(WINAPI*)(OSVERSIONINFOW*);
    OSVERSIONINFOW os{sizeof(os)};
    if (auto get = reinterpret_cast<RtlGetVersion>(
            reinterpret_cast<void*>(GetProcAddress(GetModuleHandleW(L"ntdll.dll"), "RtlGetVersion")))) {
        get(&os);
    }
    DWORD ubr = 0, size = sizeof ubr;
    RegGetValueW(HKEY_LOCAL_MACHINE, L"SOFTWARE\\Microsoft\\Windows NT\\CurrentVersion", L"UBR", RRF_RT_REG_DWORD,
                 nullptr, &ubr, &size);
    r << "Windows: " << os.dwMajorVersion << "." << os.dwMinorVersion << "." << os.dwBuildNumber << "." << ubr << " ("
      << reg_string(HKEY_LOCAL_MACHINE, L"SOFTWARE\\Microsoft\\Windows NT\\CurrentVersion", L"DisplayVersion") << ")\n";
    r << "Processor: "
      << reg_string(HKEY_LOCAL_MACHINE, L"HARDWARE\\DESCRIPTION\\System\\CentralProcessor\\0", L"ProcessorNameString")
      << " (" << std::thread::hardware_concurrency() << " threads)\n";
    MEMORYSTATUSEX mem{sizeof(mem)};
    if (GlobalMemoryStatusEx(&mem))
        r << "Memory: " << (mem.ullTotalPhys >> 20) << " MB (" << (mem.ullAvailPhys >> 20) << " MB free)\n";
    ULARGE_INTEGER free_bytes{};
    if (GetDiskFreeSpaceExW(g_root.c_str(), &free_bytes, nullptr, nullptr))
        r << "Free space on the game's drive: " << (free_bytes.QuadPart >> 30) << " GB\n";
    r << "Displays: " << GetSystemMetrics(SM_CMONITORS) << ", main " << GetSystemMetrics(SM_CXSCREEN) << "x"
      << GetSystemMetrics(SM_CYSCREEN) << " at " << GetDpiForSystem() << " dpi\n";

    r << "\nGPUs (in Windows' order):\n";
    IDXGIFactory1* factory = nullptr;
    if (SUCCEEDED(CreateDXGIFactory1(__uuidof(IDXGIFactory1), reinterpret_cast<void**>(&factory)))) {
        IDXGIAdapter1* adapter = nullptr;
        for (UINT i = 0; factory->EnumAdapters1(i, &adapter) != DXGI_ERROR_NOT_FOUND; ++i) {
            DXGI_ADAPTER_DESC1 d;
            if (SUCCEEDED(adapter->GetDesc1(&d))) {
                char ids[64];
                std::snprintf(ids, sizeof ids, "vendor 0x%04X device 0x%04X rev %u", d.VendorId, d.DeviceId, d.Revision);
                r << "  " << i << ": " << narrow(d.Description) << " (" << ids << ")";
                if (d.Flags & DXGI_ADAPTER_FLAG_SOFTWARE) r << " software";
                r << "\n     video memory " << (d.DedicatedVideoMemory >> 20) << " MB, shared "
                  << (d.SharedSystemMemory >> 20) << " MB";
                LARGE_INTEGER umd{};
                if (SUCCEEDED(adapter->CheckInterfaceSupport(__uuidof(IDXGIDevice), &umd))) {
                    r << ", driver " << HIWORD(umd.HighPart) << "." << LOWORD(umd.HighPart) << "."
                      << HIWORD(umd.LowPart) << "." << LOWORD(umd.LowPart);
                }
                r << "\n";
            }
            adapter->Release();
        }
        factory->Release();
    }
    return r.str();
}

// The graphics cards for the page (its automatic choice of GPU and settings):
// [{"name","vendor","vram" (dedicated MB)}], software adapters left out.
std::wstring gpus_json() {
    std::wstring out = L"[";
    IDXGIFactory1* factory = nullptr;
    if (SUCCEEDED(CreateDXGIFactory1(__uuidof(IDXGIFactory1), reinterpret_cast<void**>(&factory)))) {
        IDXGIAdapter1* adapter = nullptr;
        bool first = true;
        for (UINT i = 0; factory->EnumAdapters1(i, &adapter) != DXGI_ERROR_NOT_FOUND; ++i) {
            DXGI_ADAPTER_DESC1 d;
            if (SUCCEEDED(adapter->GetDesc1(&d)) && !(d.Flags & DXGI_ADAPTER_FLAG_SOFTWARE) && d.VendorId != 0x1414) {
                std::wstring name;
                for (const wchar_t* c = d.Description; *c; ++c)
                    if (*c != L'"' && *c != L'\\' && *c >= 32) name += *c;
                if (!first) out += L",";
                first = false;
                out += L"{\"name\":\"" + name + L"\",\"vendor\":" + std::to_wstring(d.VendorId) +
                       L",\"vram\":" + std::to_wstring(d.DedicatedVideoMemory >> 20) + L"}";
            }
            adapter->Release();
        }
        factory->Release();
    }
    return out + L"]";
}

// Copies `src` to `dst` with the names replaced; false when there is no `src`.
bool stage_text(const std::wstring& src, const std::wstring& dst) {
    if (!exists(src)) return false;
    write_file(dst, scrubbed(log_tail(src)));
    return true;
}

void remove_tree(const std::wstring& dir) {
    std::wstring from = dir + L'\0';  // SHFileOperation takes a double-terminated list
    SHFILEOPSTRUCTW op{};
    op.wFunc = FO_DELETE;
    op.pFrom = from.c_str();
    op.fFlags = FOF_NO_UI;
    SHFileOperationW(&op);
}

// "" when the zip is at `out`, else what went wrong.
std::wstring make_logs_zip(const std::wstring& out) {
    wchar_t tmp[MAX_PATH];
    GetTempPathW(MAX_PATH, tmp);
    const std::wstring stage = std::wstring(tmp) + L"NieR-logs-" + date_stamp();
    CreateDirectoryW(stage.c_str(), nullptr);
    CreateDirectoryW((stage + L"\\logs").c_str(), nullptr);
    CreateDirectoryW((stage + L"\\runtime-logs").c_str(), nullptr);

    write_file(stage + L"\\system.txt", scrubbed(system_report()));
    std::wstring items = L"system.txt logs runtime-logs";
    for (const wchar_t* f : {L"settings.json", L"setup.json"}) {
        if (stage_text(path(f), stage + L"\\" + f)) items += std::wstring(L" ") + f;
    }
    // The servers' and the game's logs, and the runtime's own (the arm64 translation).
    for (const auto& [from, to] : {std::pair{path(L"logs"), stage + L"\\logs"},
                                   std::pair{path(L"runtime-data\\logs"), stage + L"\\runtime-logs"}}) {
        WIN32_FIND_DATAW fd;
        HANDLE h = FindFirstFileW((from + L"\\*.log").c_str(), &fd);
        if (h == INVALID_HANDLE_VALUE) continue;
        do {
            if (!(fd.dwFileAttributes & FILE_ATTRIBUTE_DIRECTORY))
                stage_text(from + L"\\" + fd.cFileName, to + L"\\" + fd.cFileName);
        } while (FindNextFileW(h, &fd));
        FindClose(h);
    }

    // tar runs in the staging folder with relative names, and writes there:
    // the zip is moved after, so no path tar sees has characters outside ASCII.
    wchar_t sys[MAX_PATH];
    GetSystemDirectoryW(sys, MAX_PATH);
    const std::wstring tar = std::wstring(sys) + L"\\tar.exe";
    std::wstring cmd = L"\"" + tar + L"\" -a -c -f logs.zip " + items;
    STARTUPINFOW si{};
    si.cb = sizeof(si);
    PROCESS_INFORMATION pi{};
    DWORD code = 1;
    if (CreateProcessW(tar.c_str(), &cmd[0], nullptr, nullptr, FALSE, CREATE_NO_WINDOW, nullptr, stage.c_str(), &si, &pi)) {
        WaitForSingleObject(pi.hProcess, 120000);
        GetExitCodeProcess(pi.hProcess, &code);
        CloseHandle(pi.hProcess);
        CloseHandle(pi.hThread);
    }
    const bool ok = code == 0 && MoveFileExW((stage + L"\\logs.zip").c_str(), out.c_str(),
                                            MOVEFILE_REPLACE_EXISTING | MOVEFILE_COPY_ALLOWED);
    remove_tree(stage);
    if (ok) return {};
    return exists(tar) ? L"The zip could not be made." : L"Windows' tar.exe is missing.";
}

void export_logs(std::wstring out) {
    const std::wstring error = make_logs_zip(out);
    if (error.empty()) {
        ShellExecuteW(nullptr, L"open", L"explorer.exe", (L"/select,\"" + out + L"\"").c_str(), nullptr, SW_SHOWNORMAL);
        to_page(L"exported:" + out);
    } else {
        to_page(L"export-error:" + error);
    }
    g_exporting = false;
}

// ---- Settings › Saves ----------------------------------------------------------------
// Save slots and the editor of what a player owns are the server's
// (internal/pcsave): `nier-server --admin` reads one JSON request on stdin and
// writes one JSON reply, which goes to the page as "saves:<json>". Changes are
// made only while the game is closed (the server checks its port too); a copy
// of a save is kept in save\slot-backups before each change.

std::atomic<bool> g_saves{false};

void run_saves(std::string request) {
    const std::wstring bin = path(L"server\\bin"), data = path(L"data"), save = path(L"save"), logs = path(L"logs");
    CreateDirectoryW(save.c_str(), nullptr);
    CreateDirectoryW(logs.c_str(), nullptr);
    std::string reply;
    SECURITY_ATTRIBUTES sa{sizeof(sa), nullptr, TRUE};
    HANDLE in_read = nullptr, in_write = nullptr, out_read = nullptr, out_write = nullptr;
    HANDLE err = CreateFileW((logs + L"\\saves.log").c_str(), FILE_APPEND_DATA, FILE_SHARE_READ | FILE_SHARE_WRITE, &sa,
                             OPEN_ALWAYS, FILE_ATTRIBUTE_NORMAL, nullptr);
    if (CreatePipe(&in_read, &in_write, &sa, 0) && CreatePipe(&out_read, &out_write, &sa, 0)) {
        SetHandleInformation(in_write, HANDLE_FLAG_INHERIT, 0);
        SetHandleInformation(out_read, HANDLE_FLAG_INHERIT, 0);
        STARTUPINFOW si{};
        si.cb = sizeof(si);
        si.dwFlags = STARTF_USESTDHANDLES;
        si.hStdInput = in_read;
        si.hStdOutput = out_write;
        si.hStdError = err;
        const std::wstring exe = bin + L"\\nier-server.exe";
        std::wstring cmd = L"\"" + exe + L"\" --admin --save \"" + save + L"\" --data \"" + data + L"\"";
        PROCESS_INFORMATION pi{};
        if (CreateProcessW(exe.c_str(), &cmd[0], nullptr, nullptr, TRUE, CREATE_NO_WINDOW, nullptr, data.c_str(), &si, &pi)) {
            if (g_job) AssignProcessToJobObject(g_job, pi.hProcess);
            CloseHandle(pi.hThread);
            CloseHandle(in_read);
            CloseHandle(out_write);
            in_read = out_write = nullptr;
            // The request is small; the reply (the editor's lists) is read to the end.
            DWORD written = 0;
            WriteFile(in_write, request.data(), static_cast<DWORD>(request.size()), &written, nullptr);
            CloseHandle(in_write);
            in_write = nullptr;
            char buf[65536];
            DWORD got = 0;
            while (ReadFile(out_read, buf, sizeof(buf), &got, nullptr) && got > 0) reply.append(buf, got);
            WaitForSingleObject(pi.hProcess, 60000);
            CloseHandle(pi.hProcess);
        }
    }
    for (HANDLE h : {in_read, in_write, out_read, out_write, err})
        if (h && h != INVALID_HANDLE_VALUE) CloseHandle(h);
    while (!reply.empty() && (reply.back() == '\n' || reply.back() == '\r')) reply.pop_back();
    if (reply.empty() || reply[0] != '{')
        reply = "{\"ok\":false,\"error\":\"The save tool did not answer. See logs\\\\saves.log.\"}";
    g_saves = false;
    to_page(L"saves:" + widen(reply));
}

// ---- messages from the page -------------------------------------------------------

void on_page_message(const std::wstring& msg) {
    const size_t nl = msg.find(L'\n');
    const std::wstring cmd = msg.substr(0, nl);
    const std::wstring rest = nl == std::wstring::npos ? L"" : msg.substr(nl + 1);
    if (cmd == L"load") {
        to_page(L"settings:" + widen(read_file(path(L"settings.json"))));
    } else if (cmd == L"save") {
        write_file(path(L"settings.json"), narrow(rest));
    } else if (cmd == L"info") {
        const bool files = GetFileAttributesW(path(L"data\\assets\\revisions\\0\\list.bin").c_str()) != INVALID_FILE_ATTRIBUTES;
        WIN32_FILE_ATTRIBUTE_DATA fa{};
        std::wstring save;
        if (GetFileAttributesExW(path(L"save\\game.db").c_str(), GetFileExInfoStandard, &fa)) {
            const unsigned long long size = (static_cast<unsigned long long>(fa.nFileSizeHigh) << 32) | fa.nFileSizeLow;
            save = size >= (1ull << 20) ? std::to_wstring(size >> 20) + L" MB" : std::to_wstring(size >> 10) + L" KB";
        }
        MEMORYSTATUSEX mem{sizeof(mem)};
        GlobalMemoryStatusEx(&mem);
        to_page(L"info:{\"files\":" + std::wstring(files ? L"true" : L"false") + L",\"save\":\"" + save +
                L"\",\"ready\":" + std::wstring(setup_ready() ? L"true" : L"false") +
                L",\"ram\":" + std::to_wstring(mem.ullTotalPhys >> 20) + L",\"gpus\":" + gpus_json() + L"}");
    } else if (cmd == L"logs") {
        ShellExecuteW(nullptr, L"open", path(L"logs").c_str(), nullptr, nullptr, SW_SHOWNORMAL);
    } else if (cmd == L"export-logs") {
        if (g_exporting.exchange(true)) return;
        const std::wstring out = pick_export_path();
        if (out.empty()) {
            g_exporting = false;
            return;
        }
        to_page(L"exporting:");
        std::thread(export_logs, out).detach();
    } else if (cmd == L"quit") {
        PostMessageW(g_hwnd, WM_CLOSE, 0, 0);
    } else if (cmd == L"pick") {
        const std::wstring chosen = pick(rest);
        if (!chosen.empty()) to_page(L"picked:" + rest + L":" + chosen);
    } else if (cmd == L"setup") {
        if (g_busy.exchange(true)) return;
        std::wistringstream lines(rest);
        std::wstring apk, assets, master;
        std::getline(lines, apk);
        std::getline(lines, assets);
        std::getline(lines, master);
        std::thread(run_setup, apk, assets, master).detach();
    } else if (cmd == L"saves") {
        // Reading the list is fine while the game runs; changes are not. Settings ›
        // Events (content-list, content-set, images) is a choice for the next start.
        const bool listing = rest.find(L"\"op\":\"list\"") != std::wstring::npos ||
                             rest.find(L"\"op\":\"content-") != std::wstring::npos ||
                             rest.find(L"\"op\":\"images\"") != std::wstring::npos;
        if (!listing && g_busy) {
            to_page(L"saves:{\"ok\":false,\"error\":\"The game is running. Close it before changing saves.\"}");
            return;
        }
        if (g_saves.exchange(true)) {
            to_page(L"saves:{\"ok\":false,\"error\":\"Another save change is still running.\"}");
            return;
        }
        std::thread(run_saves, narrow(rest)).detach();
    } else if (cmd == L"pick-save-export") {
        // A save to the phone: where to write its backup ("picked:save-export:<path>").
        std::wstring name = rest;
        for (auto& c : name) if (wcschr(L"\\/:*?\"<>|", c)) c = L'_';
        const std::wstring out = pick_save_path(L"Export the save for the phone (Bunker's Import save backup)",
                                                L"NieR-save-" + name + L"-" + date_stamp() + L".zip");
        if (!out.empty()) to_page(L"picked:save-export:" + out);
    } else if (cmd == L"open-releases") {
        // Settings › System › Updates: the port's download page (only this address).
        ShellExecuteW(nullptr, L"open", L"https://github.com/AllenWalkerED/nier-reincarnation-pc/releases/latest", nullptr,
                      nullptr, SW_SHOWNORMAL);
    } else if (cmd == L"saves-folder") {
        const std::wstring dir = path(L"save\\slot-backups");
        CreateDirectoryW(dir.c_str(), nullptr);
        ShellExecuteW(nullptr, L"open", dir.c_str(), nullptr, nullptr, SW_SHOWNORMAL);
    } else if (cmd == L"play") {
        if (!setup_ready()) {
            to_page(L"error:Prepare the game first.");
            return;
        }
        if (g_saves) {
            to_page(L"error:Wait for the save change to finish.");
            return;
        }
        if (g_busy.exchange(true)) return;
        Launch l;
        std::wistringstream lines(rest);
        std::wstring line;
        while (std::getline(lines, line)) {
            if (line.rfind(L"input:", 0) == 0) l.input_json = narrow(line.substr(6));
            if (line.rfind(L"mods:", 0) == 0) l.mods_json = narrow(line.substr(5));
            if (line.rfind(L"pref:", 0) == 0) {
                const size_t eq = line.find(L'=');
                if (eq != std::wstring::npos) l.prefs.emplace_back(narrow(line.substr(5, eq - 5)), narrow(line.substr(eq + 1)));
            }
            if (line.rfind(L"env:", 0) == 0) {
                const size_t eq = line.find(L'=');
                if (eq != std::wstring::npos) l.env.emplace_back(line.substr(4, eq - 4), line.substr(eq + 1));
            }
        }
        std::thread(run_game, std::move(l)).detach();
    }
}

void create_webview() {
    using CreateEnv = HRESULT(STDAPICALLTYPE*)(PCWSTR, PCWSTR, ICoreWebView2EnvironmentOptions*,
                                                ICoreWebView2CreateCoreWebView2EnvironmentCompletedHandler*);
    HMODULE loader = LoadLibraryW(path(L"WebView2Loader.dll").c_str());
    auto create = loader ? reinterpret_cast<CreateEnv>(reinterpret_cast<void*>(
                               GetProcAddress(loader, "CreateCoreWebView2EnvironmentWithOptions")))
                         : nullptr;
    if (!create) {
        MessageBoxW(g_hwnd, L"WebView2Loader.dll is missing next to NieR.exe.", L"NieR", MB_ICONERROR);
        return;
    }
    const std::wstring profile = path(L"launcher\\webview-data");
    create(nullptr, profile.c_str(), nullptr,
           make_handler<ICoreWebView2CreateCoreWebView2EnvironmentCompletedHandler>(
               IID_ICoreWebView2CreateCoreWebView2EnvironmentCompletedHandler,
               [](HRESULT hr, ICoreWebView2Environment* env) -> HRESULT {
                   if (FAILED(hr) || !env) {
                       MessageBoxW(g_hwnd, L"The WebView2 Runtime is not available.", L"NieR", MB_ICONERROR);
                       return S_OK;
                   }
                   env->CreateCoreWebView2Controller(
                       g_hwnd, make_handler<ICoreWebView2CreateCoreWebView2ControllerCompletedHandler>(
                                   IID_ICoreWebView2CreateCoreWebView2ControllerCompletedHandler,
                                   [](HRESULT hr2, ICoreWebView2Controller* controller) -> HRESULT {
                                       if (FAILED(hr2) || !controller) return S_OK;
                                       g_controller.retain(controller);
                                       g_controller->get_CoreWebView2(g_webview.put());
                                       ComPtr<ICoreWebView2Controller2> c2;
                                       g_controller->QueryInterface(IID_ICoreWebView2Controller2,
                                                                    reinterpret_cast<void**>(c2.put()));
                                       if (c2) c2->put_DefaultBackgroundColor(COREWEBVIEW2_COLOR{255, 12, 11, 10});
                                       RECT r;
                                       GetClientRect(g_hwnd, &r);
                                       g_controller->put_Bounds(r);
                                       ComPtr<ICoreWebView2Settings> settings;
                                       g_webview->get_Settings(settings.put());
                                       if (settings) {
                                           settings->put_AreDevToolsEnabled(FALSE);
                                           settings->put_AreDefaultContextMenusEnabled(FALSE);
                                           settings->put_IsStatusBarEnabled(FALSE);
                                           settings->put_IsZoomControlEnabled(FALSE);
                                       }
                                       EventRegistrationToken token;
                                       g_webview->add_WebMessageReceived(
                                           make_handler<ICoreWebView2WebMessageReceivedEventHandler>(
                                               IID_ICoreWebView2WebMessageReceivedEventHandler,
                                               [](ICoreWebView2*, ICoreWebView2WebMessageReceivedEventArgs* args)
                                                   -> HRESULT {
                                                   LPWSTR text = nullptr;
                                                   if (SUCCEEDED(args->TryGetWebMessageAsString(&text)) && text) {
                                                       on_page_message(text);
                                                       CoTaskMemFree(text);
                                                   }
                                                   return S_OK;
                                               }),
                                           &token);
                                       ComPtr<ICoreWebView2_3> w3;
                                       g_webview->QueryInterface(IID_ICoreWebView2_3, reinterpret_cast<void**>(w3.put()));
                                       if (w3) {
                                           w3->SetVirtualHostNameToFolderMapping(
                                               L"nier.launcher", path(L"launcher\\ui").c_str(),
                                               COREWEBVIEW2_HOST_RESOURCE_ACCESS_KIND_DENY_CORS);
                                       }
                                       g_webview->Navigate(L"https://nier.launcher/index.html");
                                       return S_OK;
                                   }));
                   return S_OK;
               }));
}

LRESULT CALLBACK window_proc(HWND hwnd, UINT msg, WPARAM wp, LPARAM lp) {
    switch (msg) {
        case WM_SIZE:
            if (g_controller) {
                RECT r;
                GetClientRect(hwnd, &r);
                g_controller->put_Bounds(r);
            }
            return 0;
        case WM_GETMINMAXINFO: {
            auto* mm = reinterpret_cast<MINMAXINFO*>(lp);
            mm->ptMinTrackSize = POINT{960, 600};
            return 0;
        }
        case kMsgToPage: {
            auto* text = reinterpret_cast<std::wstring*>(lp);
            if (g_webview) g_webview->PostWebMessageAsString(text->c_str());
            delete text;
            return 0;
        }
        case kMsgShow:
            ShowWindow(hwnd, wp ? SW_SHOW : SW_HIDE);
            if (wp) SetForegroundWindow(hwnd);
            return 0;
        case WM_CLOSE:
            DestroyWindow(hwnd);
            return 0;
        case WM_DESTROY:
            PostQuitMessage(0);
            return 0;
        default:
            return DefWindowProcW(hwnd, msg, wp, lp);
    }
}

}  // namespace

int WINAPI wWinMain(HINSTANCE instance, HINSTANCE, PWSTR, int show) {
    wchar_t exe[MAX_PATH * 2];
    const DWORD n = GetModuleFileNameW(nullptr, exe, MAX_PATH * 2);
    g_root.assign(exe, n);
    g_root.resize(g_root.find_last_of(L"\\/"));

    // NieR.exe --export-logs <file.zip>: the Export logs zip without the window
    // (exit code 0 when made), for when the launcher itself does not open.
    int argc = 0;
    if (wchar_t** argv = CommandLineToArgvW(GetCommandLineW(), &argc)) {
        if (argc == 3 && wcscmp(argv[1], L"--export-logs") == 0) {
            CoInitializeEx(nullptr, COINIT_APARTMENTTHREADED);
            wchar_t full[MAX_PATH * 2];
            GetFullPathNameW(argv[2], MAX_PATH * 2, full, nullptr);
            LocalFree(argv);
            return make_logs_zip(full).empty() ? 0 : 1;
        }
        LocalFree(argv);
    }

    // One launcher at a time.
    HANDLE once = CreateMutexW(nullptr, TRUE, L"NieRReincarnationLauncher");
    if (GetLastError() == ERROR_ALREADY_EXISTS) {
        if (HWND other = FindWindowW(L"NieRLauncher", nullptr)) {
            ShowWindow(other, SW_SHOW);
            SetForegroundWindow(other);
        }
        return 0;
    }

    WSADATA wsa;
    WSAStartup(MAKEWORD(2, 2), &wsa);
    CoInitializeEx(nullptr, COINIT_APARTMENTTHREADED);
    SetProcessDpiAwarenessContext(DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2);
    g_job = make_job();

    WNDCLASSEXW wc{};
    wc.cbSize = sizeof(wc);
    wc.lpfnWndProc = window_proc;
    wc.hInstance = instance;
    wc.hCursor = LoadCursorW(nullptr, MAKEINTRESOURCEW(32512));
    wc.hIcon = LoadIconW(instance, MAKEINTRESOURCEW(1));
    wc.hIconSm = wc.hIcon;
    wc.hbrBackground = CreateSolidBrush(RGB(12, 11, 10));
    wc.lpszClassName = L"NieRLauncher";
    RegisterClassExW(&wc);

    const UINT dpi = GetDpiForSystem();
    const int w = MulDiv(1280, static_cast<int>(dpi), 96), h = MulDiv(760, static_cast<int>(dpi), 96);
    g_hwnd = CreateWindowExW(0, wc.lpszClassName, L"NieR Re[in]carnation", WS_OVERLAPPEDWINDOW, CW_USEDEFAULT,
                             CW_USEDEFAULT, w, h, nullptr, nullptr, instance, nullptr);
    const BOOL dark = TRUE;
    DwmSetWindowAttribute(g_hwnd, 20 /* DWMWA_USE_IMMERSIVE_DARK_MODE */, &dark, sizeof(dark));
    ShowWindow(g_hwnd, show);
    create_webview();

    MSG m;
    while (GetMessageW(&m, nullptr, 0, 0) > 0) {
        TranslateMessage(&m);
        DispatchMessageW(&m);
    }
    // Closing the launcher ends the servers and a game it started (the job).
    if (g_job) CloseHandle(g_job);
    CloseHandle(once);
    return 0;
}
