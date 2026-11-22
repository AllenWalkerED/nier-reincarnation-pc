"""Stand-in for fmod_toolkit, which bundles FMOD (not redistributable here).
UnityPy imports it at load; the port never converts audio."""


def __getattr__(name):
    raise RuntimeError("Audio conversion is not available in this port (no FMOD)")
