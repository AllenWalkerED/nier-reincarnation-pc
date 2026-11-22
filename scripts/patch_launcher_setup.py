"""Adds the first-run setup to NieR.exe (launcher/src/main.cpp): the page asks
for the player's game files, NieR.exe runs tools/setup_game.py with the
bundled Python and passes its progress lines on, and the game starts only
once setup.json says the port is prepared."""
import sys

p = sys.argv[1]
s = open(p, encoding='utf-8', newline='').read()


def rep(old, new):
    global s
    assert s.count(old) == 1, (old[:70], s.count(old))
    s = s.replace(old, new)


rep('''#include "WebView2.h"''', '''#include <shobjidl.h>

#include "WebView2.h"''')

rep('''// ---- messages from the page -------------------------------------------------------
''', r'''// ---- first-run setup ---------------------------------------------------------------
// The port ships without the game: tools/setup_game.py prepares it from the
// player's files (setup_game.py says what it makes). Ready when its record
// and what it made are there.

bool exists(const std::wstring& p) { return GetFileAttributesW(p.c_str()) != INVALID_FILE_ATTRIBUTES; }

bool setup_ready() {
    return exists(path(L"setup.json")) && exists(path(L"client\\NieR-loopback.apk")) &&
           exists(path(L"data\\assets\\revisions\\0\\list.bin")) && exists(path(L"runtime-data\\android"));
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
        static const COMDLG_FILTERSPEC f[] = {{L"Asset dump (.7z)", L"*.7z"}};
        dlg->SetFileTypes(1, f);
        dlg->SetTitle(L"resource_dump_android.7z");
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

// ---- messages from the page -------------------------------------------------------
''')

rep('''        to_page(L"info:{\\"files\\":" + std::wstring(files ? L"true" : L"false") + L",\\"save\\":\\"" + save + L"\\"}");''',
    '''        to_page(L"info:{\\"files\\":" + std::wstring(files ? L"true" : L"false") + L",\\"save\\":\\"" + save +
                L"\\",\\"ready\\":" + std::wstring(setup_ready() ? L"true" : L"false") + L"}");''')

rep('''    } else if (cmd == L"play") {''', r'''    } else if (cmd == L"pick") {
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
    } else if (cmd == L"play") {
        if (!setup_ready()) {
            to_page(L"error:Prepare the game first.");
            return;
        }''')
open(p, 'w', encoding='utf-8', newline='').write(s)
print('ok')
