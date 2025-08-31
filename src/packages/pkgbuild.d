module packages.pkgbuild;

import std.algorithm;
import std.array;
import std.file;
import std.format;
import std.net.curl;
import std.path;
import std.process;
import std.stdio;
import std.string;
import std.typecons;
import terminal.colors;

/// Get PKGBUILD content from AUR
string fetchPkgbuildFromAur(string packageName)
{
    try
    {
        string url = format("https://aur.archlinux.org/cgit/aur.git/plain/PKGBUILD?h=%s",
                packageName);
        return get(url).idup;
    }
    catch (Exception e)
    {
        return "";
    }
}

/// Get local PKGBUILD content from a directory
string getLocalPkgbuild(string dir)
{
    string pkgbuildPath = buildPath(dir, "PKGBUILD");
    if (exists(pkgbuildPath))
    {
        try
        {
            return readText(pkgbuildPath);
        }
        catch (Exception e)
        {
            return "";
        }
    }
    return "";
}

/// Generate diff between two PKGBUILD strings
string diffPkgbuild(string local, string remote)
{
    if (local.length == 0 && remote.length == 0)
    {
        return "No PKGBUILD content available for comparison";
    }

    if (local.length == 0)
    {
        return "Local PKGBUILD not found, will use remote version";
    }

    if (remote.length == 0)
    {
        return "Remote PKGBUILD not available, using local version";
    }

    // Simple line-by-line comparison
    auto localLines = local.splitLines();
    auto remoteLines = remote.splitLines();

    if (localLines == remoteLines)
    {
        return "PKGBUILDs are identical";
    }

    // Generate a basic diff output
    string result = bold("PKGBUILD Diff:") ~ "\n";
    result ~= "--- Local\n";
    result ~= "+++ Remote\n";

    size_t maxLines = max(localLines.length, remoteLines.length);
    for (size_t i = 0; i < maxLines; i++)
    {
        string localLine = i < localLines.length ? localLines[i] : "";
        string remoteLine = i < remoteLines.length ? remoteLines[i] : "";

        if (localLine != remoteLine)
        {
            if (localLine.length > 0)
            {
                result ~= red("- " ~ localLine) ~ "\n";
            }
            if (remoteLine.length > 0)
            {
                result ~= green("+ " ~ remoteLine) ~ "\n";
            }
        }
    }

    return result;
}

/// Get PKGBUILD for comparison (local and remote)
Tuple!(string, string) getPkgbuild(string packageName)
{
    // For now, return empty strings as we don't have a local directory context
    // This would be called from within the build directory
    string remote = fetchPkgbuildFromAur(packageName);
    return tuple("", remote);
}

/// Get PKGBUILD diff for a package in a specific directory
string getPkgbuildDiff(string packageName, string localDir)
{
    string local = getLocalPkgbuild(localDir);
    string remote = fetchPkgbuildFromAur(packageName);
    return diffPkgbuild(local, remote);
}
