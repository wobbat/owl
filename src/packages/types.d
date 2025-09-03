module packages.types;

import std.string : endsWith;

enum PackageSource
{
    any,
    repo,
    aur
}

enum PackageActionStatus
{
    install,
    remove
}

struct PackageAction
{
    string name;
    PackageActionStatus status;
}

struct OutdatedPackage
{
    string name;
    string fromVersion;
    string toVersion;
    string source; // repo name or "aur"
}

struct PackageInfo
{
    string name;
    string ver;
    PackageSource source;
    string repo;
    string description;
    bool installed;
    bool explicit;
}

struct SearchResult
{
    string name;
    string ver;
    PackageSource source;
    string repo;
    string description;
    bool installed;
}

struct PMOptions
{
    bool unsafe;
    bool bypassCache;
    bool noAur;
    bool verbose;
}

alias ProgressCallback = void delegate(string msg);

/// Check if a package is a VCS package (ends with -git, -hg, etc.)
bool isVCSPackage(string name)
{
    return name.endsWith("-git") || name.endsWith("-hg")
        || name.endsWith("-svn") || name.endsWith("-bzr");
}

/// Parse pacman output into name-version pairs
string[2][] parsePacmanOutput(string output)
{
    import std.string : splitLines, split;

    string[2][] packages;
    foreach (line; output.splitLines())
    {
        if (line.length == 0)
            continue;
        auto parts = line.split();
        if (parts.length >= 2)
        {
            packages ~= [parts[0], parts[1]];
        }
    }
    return packages;
}
