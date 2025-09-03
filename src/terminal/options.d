module terminal.options;

import packages.types;

struct CommandOptions
{
    // Global toggles
    bool noSpinner;
    bool verbose;
    bool debugMode;
    bool dev; // renamed from devel to match yay --devel -> --dev
    bool bypassCache;
    bool unsafe;
    bool safety;
    bool noAur;
    bool sync;
    bool paru; // use paru instead of pacman when available

    // Common flags
    string exact;
    string file;
    PackageSource source = PackageSource.any;
    bool yes;
    bool json;
    bool all;
    bool dryRun;
    int limit;
}

struct ParsedCommand
{
    string[] args;
    CommandOptions opts;
}

import std.string : startsWith;
import std.conv : to;
import std.algorithm : canFind;

CommandOptions parseCommandOptions(const bool[string] flags, const string[] arguments)
{
    CommandOptions opts;

    // Set defaults
    opts.paru = true;

    // Parse flags
    if ("no-spinner" in flags)
        opts.noSpinner = true;
    if ("verbose" in flags)
        opts.verbose = true;
    if ("debug" in flags)
        opts.debugMode = true;
    if ("dev" in flags)
        opts.dev = true;
    if ("bypass-cache" in flags)
        opts.bypassCache = true;
    if ("unsafe" in flags)
        opts.unsafe = true;
    if ("safety" in flags)
        opts.safety = true;
    if ("no-aur" in flags)
        opts.noAur = true;
    if ("sync" in flags)
        opts.sync = true;
    if ("paru" in flags)
        opts.paru = true;
    if ("no-paru" in flags)
        opts.paru = false;
    if ("yes" in flags)
        opts.yes = true;
    if ("json" in flags)
        opts.json = true;
    if ("all" in flags)
        opts.all = true;
    if ("dry-run" in flags)
        opts.dryRun = true;

    // Parse source flag
    if ("source-repo" in flags)
        opts.source = PackageSource.repo;
    else if ("source-aur" in flags)
        opts.source = PackageSource.aur;
    else
        opts.source = PackageSource.any;

    return opts;
}
