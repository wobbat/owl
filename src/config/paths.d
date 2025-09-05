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
    UntrackedState = "~/.owl/.state/untracked.json",
    HiddenState = "~/.owl/.state/hidden.txt",

    // Dotfiles
    DotfilesDir = "~/.owl/dotfiles"
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



