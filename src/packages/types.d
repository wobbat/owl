module packages.types;

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
