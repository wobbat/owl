module config.paths;

import std.path : buildPath, expandTilde;

// Central path configuration for the Owl package manager
enum OwlPaths : string
{
    // Main configuration directory
    ConfigRoot = "~/.owl",

    // Configuration files
    MainConfig = "~/.owl/main.owl",

    // State directory and files
    StateDir = "~/.owl/.state",
    ManagedState = "~/.owl/.state/managed.json",
    GlobalEnvState = "~/.owl/.state/global-env.json",
    UntrackedState = "~/.owl/.state/untracked.json",
    HiddenState = "~/.owl/.state/hidden.txt",

    // Dotfiles
    DotfilesDir = "~/.owl/dotfiles"
}

// Helper functions for path operations
string owlPath(OwlPaths path)
{
    return expandTilde(cast(string) path);
}

string owlConfigRoot()
{
    return expandTilde(OwlPaths.ConfigRoot);
}

string owlMainConfig()
{
    return expandTilde(OwlPaths.MainConfig);
}

string owlStateDir()
{
    return expandTilde(OwlPaths.StateDir);
}

string owlManagedState()
{
    return expandTilde(OwlPaths.ManagedState);
}

string owlGlobalEnvState()
{
    return expandTilde(OwlPaths.GlobalEnvState);
}

string owlUntrackedState()
{
    return expandTilde(OwlPaths.UntrackedState);
}

string owlHiddenState()
{
    return expandTilde(OwlPaths.HiddenState);
}

string owlDotfilesDir()
{
    return expandTilde(OwlPaths.DotfilesDir);
}

// Host-specific config path helper
string owlHostConfig(string hostname)
{
    return buildPath(owlConfigRoot(), hostname ~ ".owl");
}

// Group config path helper
string owlGroupConfig(string groupName)
{
    return buildPath(owlConfigRoot(), "groups", groupName ~ ".owl");
}

// Hosts directory config path helper
string owlHostsDirConfig(string hostname)
{
    return buildPath(owlConfigRoot(), "hosts", hostname ~ ".owl");
}
