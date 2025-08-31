module packages.state;

import std.algorithm;
import std.array;
import std.conv;
import std.file;
import std.json;
import std.path;
import std.stdio;
import std.string;
import std.typecons;
import packages.pacman;
import config.loader;
import config.paths;

/// Default system packages that should not be tracked
string[] defaultUntrackedSeed()
{
    return [
        "linux", "linux-firmware", "intel-ucode", "amd-ucode", "base",
        "base-devel", "glibc", "filesystem", "bash", "coreutils", "findutils",
        "grep", "gawk", "sed", "less", "util-linux", "procps-ng", "shadow",
        "iproute2", "iputils", "pacman", "pacman-contrib", "gzip", "xz",
        "tar", "openssl", "ca-certificates", "e2fsprogs"
    ];
}

/// Get path to untracked packages JSON file
string untrackedPath()
{
    return owlUntrackedState();
}

/// Get path to hidden packages text file
string hiddenPath()
{
    return owlHiddenState();
}

/// Write untracked packages to JSON file
void writeUntracked(string[] packages)
{
    string path = untrackedPath();
    mkdirRecurse(dirName(path));

    auto sortedPackages = packages.dup.sort().array;
    JSONValue json = JSONValue(sortedPackages);

    std.file.write(path, json.toPrettyString());
}

/// Read untracked packages from JSON file
string[] readUntracked()
{
    string path = untrackedPath();
    if (!exists(path))
    {
        auto seed = defaultUntrackedSeed();
        writeUntracked(seed);
        return seed;
    }

    try
    {
        string content = readText(path);
        JSONValue json = parseJSON(content);

        string[] result;
        foreach (item; json.array)
        {
            result ~= item.str;
        }
        return result;
    }
    catch (Exception e)
    {
        return defaultUntrackedSeed();
    }
}

/// Add package to untracked list
void addToUntracked(string packageName)
{
    auto current = readUntracked();
    if (!current.canFind(packageName))
    {
        current ~= packageName;
        writeUntracked(current);
    }
}

/// Remove package from untracked list
void removeFromUntracked(string packageName)
{
    auto current = readUntracked();
    auto filtered = current.filter!(pkg => pkg != packageName).array;
    writeUntracked(filtered);
}

/// Read hidden packages from text file
string[] readHidden()
{
    string path = hiddenPath();
    if (!exists(path))
    {
        return [];
    }

    string[] result;
    try
    {
        auto file = File(path, "r");
        foreach (line; file.byLine())
        {
            string trimmed = line.strip().idup;
            if (trimmed.length > 0)
            {
                result ~= trimmed;
            }
        }
    }
    catch (Exception e)
    {
        return [];
    }

    return result;
}

/// Add package to hidden list
void addHidden(string name)
{
    auto all = readHidden();
    if (!all.canFind(name))
    {
        all ~= name;
        writeHidden(all);
    }
}

/// Remove package from hidden list
void removeHidden(string name)
{
    auto all = readHidden().filter!(pkg => pkg != name).array;
    writeHidden(all);
}

/// Write hidden packages to text file
void writeHidden(string[] packages)
{
    string path = hiddenPath();
    mkdirRecurse(dirName(path));

    auto file = File(path, "w");
    foreach (pkg; packages)
    {
        file.writeln(pkg);
    }
}

/// Get list of explicitly installed packages
string[] listExplicitlyInstalled()
{
    auto packages = getExplicitlyInstalledPackages();
    return packages.map!(p => p[0]).array;
}

/// Compute candidates for tracking (explicitly installed but not managed or hidden)
string[] computeTrackCandidates(string host)
{
    auto explicit = listExplicitlyInstalled();
    auto conf = loadConfigChain(owlConfigRoot(), host);

    // Get managed packages from config
    bool[string] managed;
    foreach (entry; conf.entries)
    {
        if (entry.pkgName.length > 0 && !entry.pkgName.startsWith("__"))
        {
            managed[entry.pkgName] = true;
        }
    }

    // Get untracked packages
    bool[string] untracked;
    foreach (pkg; readUntracked())
    {
        untracked[pkg] = true;
    }

    // Filter to get candidates
    string[] result;
    foreach (pkg; explicit)
    {
        if (pkg !in managed && pkg !in untracked)
        {
            result ~= pkg;
        }
    }

    return result.sort().array;
}
