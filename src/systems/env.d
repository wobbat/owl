module systems.env;

import std.file;
import std.path;
import std.process;
import std.array;
import std.string;
import std.format;
import std.algorithm;
import std.conv;

/// Get the Owl directory path
string owlDir()
{
    return buildPath(environment["HOME"], ".owl");
}

/// Get bash environment file path
string envFileBash()
{
    return buildPath(owlDir(), "env.sh");
}

/// Get fish environment file path
string envFileFish()
{
    return buildPath(owlDir(), "env.fish");
}

/// Ensure Owl directories exist
void ensureOwlDirectories()
{
    string dir = owlDir();
    if (!exists(dir))
    {
        mkdirRecurse(dir);
    }
}

/// Read existing environment variables from bash file
string[string] readExistingEnvVars()
{
    string[string] existing;
    string bashFile = envFileBash();

    if (!exists(bashFile))
        return existing;

    try
    {
        auto content = readText(bashFile);
        foreach (line; content.splitLines())
        {
            line = line.strip();
            if (line.startsWith("export ") && line.canFind("="))
            {
                auto exportRemoved = line[7 .. $]; // Remove "export "
                auto eqIndex = exportRemoved.indexOf('=');
                if (eqIndex > 0)
                {
                    string key = exportRemoved[0 .. eqIndex];
                    string value = exportRemoved[eqIndex + 1 .. $];
                    // Remove quotes if present
                    if (value.startsWith("'") && value.endsWith("'") && value.length >= 2)
                    {
                        value = value[1 .. $ - 1];
                        // Unescape bash quotes
                        value = value.replace("'\\''", "'");
                    }
                    existing[key] = value;
                }
            }
        }
    }
    catch (Exception e)
    {
        // If we can't read the file, assume no existing vars
    }

    return existing;
}

/// Compare environment variables and return categorized results
struct EnvComparison
{
    string[2][] added;
    string[2][] updated;
    string[2][] unchanged;
    string[] removed;
}

EnvComparison compareEnvVars(string[2][] newEnvs)
{
    EnvComparison result;
    auto existing = readExistingEnvVars();

    // Check new/updated vars
    foreach (env; newEnvs)
    {
        string key = env[0];
        string value = env[1];

        if (key in existing)
        {
            if (existing[key] == value)
            {
                result.unchanged ~= env;
            }
            else
            {
                result.updated ~= env;
            }
        }
        else
        {
            result.added ~= env;
        }
    }

    // Check removed vars
    bool[string] newKeys;
    foreach (env; newEnvs)
    {
        newKeys[env[0]] = true;
    }

    foreach (key, value; existing)
    {
        if (key !in newKeys)
        {
            result.removed ~= key;
        }
    }

    return result;
}

/// Write bash environment file
void writeEnvBash(string[2][] envs)
{
    string content = "#!/bin/bash\n";
    content ~= "# This file is managed by Owl package manager\n";
    content ~= "# Manual changes may be overwritten\n";

    if (envs.length > 0)
    {
        content ~= "\n";
        foreach (env; envs)
        {
            string key = env[0];
            string value = env[1];
            // Escape single quotes in bash
            string escaped = value.replace("'", "'\\''");
            content ~= format("export %s='%s'\n", key, escaped);
        }
    }

    std.file.write(envFileBash(), content);
}

/// Write fish environment file
void writeEnvFish(string[2][] envs)
{
    string content = "# This file is managed by Owl package manager\n";
    content ~= "# Manual changes may be overwritten";

    if (envs.length > 0)
    {
        content ~= "\n\n";
        foreach (env; envs)
        {
            string key = env[0];
            string value = env[1];
            // Escape single quotes in fish
            string escaped = value.replace("'", "\\'");
            content ~= format("set -x %s '%s'\n", key, escaped);
        }
    }

    std.file.write(envFileFish(), content);
}

/// Set environment variables by writing both shell files
void setEnvironmentVariables(string[2][] pairs)
{
    ensureOwlDirectories();
    writeEnvBash(pairs);
    writeEnvFish(pairs);
}

/// Remove environment variables (handled by rebuilding files from current env set)
void removeEnvironmentVariables(string[2][] pairs)
{
    // This is handled by rebuilding files from the current environment set
    // The caller should provide the complete set without the variables to remove
}
