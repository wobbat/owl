module systems.dotfiles;

import std.algorithm;
import std.array;
import std.file;
import std.path;
import std.stdio;
import std.string;
import std.digest.sha;
import std.conv;
import std.json;
import std.range;
import terminal.ui;
import terminal.colors;

struct DotfileMapping
{
    string source;
    string dest;
}

struct DotfileAction
{
    string source;
    string destination;
    string status; // "create", "update", "conflict", "up-to-date", "skip"
    string reason;
    bool updateLock;
}

struct OwlLock
{
    string[string] configs;
    string[string] setups;
}

/// Get the owl state directory path
string owlStateDir()
{
    import std.process : environment;

    return buildPath(environment["HOME"], ".owl", ".state");
}

/// Get the owl lock file path
string owlLockPath()
{
    return buildPath(owlStateDir(), "owl.lock");
}

/// Get the dotfiles directory path
string getDotfilesDir()
{
    import std.process : environment;

    return buildPath(environment["HOME"], ".owl", "dotfiles");
}

/// SHA256 hash function
string simpleHash(string data)
{
    auto hashBytes = sha256Of(data);
    string hash = hashBytes.toHexString().toLower.idup;
    return hash; // Return full SHA256 hash
}

/// Hash a file's content
string hashFile(string path)
{
    if (!exists(path) || !isFile(path))
        return "";

    try
    {
        string content = readText(path);
        return simpleHash(content);
    }
    catch (Exception)
    {
        return "";
    }
}

/// Hash a directory's content recursively
string hashDir(string path)
{
    if (!exists(path) || !isDir(path))
        return "";

    string[] entries;

    try
    {
        foreach (entry; dirEntries(path, SpanMode.depth, false))
        {
            if (entry.isFile)
            {
                string relPath = relativePath(entry.name, path).replace("\\", "/");
                string fileHash = hashFile(entry.name);
                if (fileHash.length > 0)
                {
                    entries ~= relPath ~ ":" ~ fileHash;
                }
            }
        }

        // Sort entries for deterministic hash
        entries.sort();
        return simpleHash(entries.join("\n"));
    }
    catch (Exception)
    {
        return "";
    }
}

/// Hash a path (file or directory)
string hashPath(string path)
{
    if (!exists(path))
        return "";

    if (isDir(path))
        return hashDir(path);
    else
        return hashFile(path);
}

/// Load the owl lock file
OwlLock loadOwlLock()
{
    OwlLock result;
    string lockPath = owlLockPath();

    if (!exists(lockPath))
        return result;

    try
    {
        string content = readText(lockPath);
        JSONValue json = parseJSON(content);

        if ("configs" in json && json["configs"].type == JSONType.object)
        {
            foreach (key, value; json["configs"].object)
            {
                result.configs[key] = value.str;
            }
        }

        if ("setups" in json && json["setups"].type == JSONType.object)
        {
            foreach (key, value; json["setups"].object)
            {
                result.setups[key] = value.str;
            }
        }
    }
    catch (Exception e)
    {
        // If lock file is corrupted, start fresh
    }

    return result;
}

/// Save the owl lock file
void saveOwlLock(OwlLock lock)
{
    string stateDir = owlStateDir();
    if (!exists(stateDir))
    {
        mkdirRecurse(stateDir);
    }

    JSONValue json = JSONValue.emptyObject;
    json["configs"] = JSONValue.emptyObject;
    json["setups"] = JSONValue.emptyObject;

    foreach (key, value; lock.configs)
    {
        json["configs"][key] = JSONValue(value);
    }

    foreach (key, value; lock.setups)
    {
        json["setups"][key] = JSONValue(value);
    }

    try
    {
        std.file.write(owlLockPath(), json.toPrettyString());
    }
    catch (Exception e)
    {
        // Ignore write errors for now
    }
}

/// Resolve source path relative to dotfiles directory if not absolute
string resolveSourcePath(string source)
{
    if (source.startsWith("/") || source.startsWith("./") || source.startsWith("../"))
    {
        return source;
    }
    return buildPath(getDotfilesDir(), source);
}

/// Resolve destination path with tilde expansion
string resolveDestinationPath(string dest)
{
    return expandTilde(dest);
}

/// Remove a path (file or directory) safely
void removePathSafely(string path)
{
    if (!exists(path))
        return;

    try
    {
        if (isDir(path))
        {
            rmdirRecurse(path);
        }
        else
        {
            remove(path);
        }
    }
    catch (Exception e)
    {
        // Ignore removal errors
    }
}

/// Copy a path (file or directory)
void copyPath(string src, string dest)
{
    // Ensure parent directory exists
    string destDir = dirName(dest);
    if (!exists(destDir))
    {
        mkdirRecurse(destDir);
    }

    // Remove existing destination
    removePathSafely(dest);

    if (isDir(src))
    {
        copyRecursive(src, dest);
    }
    else
    {
        copy(src, dest);
    }
}

/// Analyze what actions need to be taken for dotfiles (with lock awareness)
DotfileAction[] analyzeDotfiles(DotfileMapping[] mappings)
{
    DotfileAction[] actions;
    OwlLock lock = loadOwlLock();

    foreach (mapping; mappings)
    {
        string sourcePath = resolveSourcePath(mapping.source);
        string destPath = resolveDestinationPath(mapping.dest);

        DotfileAction action;
        action.source = mapping.source;
        action.destination = mapping.dest;
        action.updateLock = false;

        // Check if source exists
        if (!exists(sourcePath))
        {
            action.status = "conflict";
            action.reason = "source file not found";
            actions ~= action;
            continue;
        }

        // Check if destination exists
        if (!exists(destPath))
        {
            action.status = "create";
            actions ~= action;
            continue;
        }

        // Check if destination is a file vs directory mismatch
        if (isFile(sourcePath) && isDir(destPath))
        {
            action.status = "conflict";
            action.reason = "destination is directory, source is file";
            actions ~= action;
            continue;
        }

        if (isDir(sourcePath) && isFile(destPath))
        {
            action.status = "conflict";
            action.reason = "destination is file, source is directory";
            actions ~= action;
            continue;
        }

        // Get current source hash
        string currentSourceHash = hashPath(sourcePath);

        // Check if we have a previous hash for this destination
        string lastAppliedHash = lock.configs.get(mapping.dest, "");

        if (lastAppliedHash.length > 0 && lastAppliedHash == currentSourceHash)
        {
            // Source hasn't changed since last apply
            action.status = "skip";
            action.reason = "No changes detected";
            actions ~= action;
            continue;
        }

        // Source has changed (or first time) - check if destination matches current source
        string currentDestHash = hashPath(destPath);

        if (currentDestHash.length > 0 && currentDestHash == currentSourceHash)
        {
            // Destination already matches source, just update lock
            action.status = "skip";
            action.reason = "Destination matches source";
            action.updateLock = true;
            actions ~= action;
            continue;
        }

        // Need to update destination
        action.status = "update";
        action.reason = "Changes detected or first-time setup";
        actions ~= action;
    }

    return actions;
}

/// Apply dotfile actions (actually copy/link files)
DotfileAction[] applyDotfiles(DotfileMapping[] mappings)
{
    if (mappings.length == 0)
        return [];

    OwlLock lock = loadOwlLock();
    bool lockUpdated = false;
    DotfileAction[] result;

    DotfileAction[] planned = analyzeDotfiles(mappings);

    foreach (action; planned)
    {
        if (action.status == "conflict" || action.status == "skip")
        {
            if (action.updateLock)
            {
                string srcAbs = resolveSourcePath(action.source);
                string newHash = hashPath(srcAbs);
                if (newHash.length > 0)
                {
                    lock.configs[action.destination] = newHash;
                    lockUpdated = true;
                }
            }
            result ~= action;
            continue;
        }

        // create or update -> copy and update lock
        string srcAbs = resolveSourcePath(action.source);
        string dstAbs = resolveDestinationPath(action.destination);

        try
        {
            copyPath(srcAbs, dstAbs);
            string newHash = hashPath(srcAbs);
            if (newHash.length > 0)
            {
                lock.configs[action.destination] = newHash;
                lockUpdated = true;
            }
            result ~= action;
        }
        catch (Exception e)
        {
            DotfileAction failed = action;
            failed.status = "conflict";
            failed.reason = "Copy failed: " ~ e.msg;
            result ~= failed;
        }
    }

    if (lockUpdated)
    {
        saveOwlLock(lock);
    }

    return result;
}

/// Recursively copy directory contents
void copyRecursive(string src, string dest)
{
    if (!exists(dest))
    {
        mkdirRecurse(dest);
    }

    foreach (entry; dirEntries(src, SpanMode.shallow))
    {
        string basename = baseName(entry.name);
        string destPath = buildPath(dest, basename);

        if (entry.isDir)
        {
            copyRecursive(entry.name, destPath);
        }
        else if (entry.isFile)
        {
            copy(entry.name, destPath);
        }
    }
}

/// Check if any dotfile mappings have actionable status
bool hasActionableDotfiles(DotfileMapping[] mappings)
{
    auto actions = analyzeDotfiles(mappings);
    return actions.any!(a => a.status == "create" || a.status == "update" || a.status == "conflict");
}
