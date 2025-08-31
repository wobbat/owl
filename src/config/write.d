module config.write;

import std.algorithm;
import std.array;
import std.file;
import std.path;
import std.process;
import std.stdio;
import std.string;
import config.loader;
import config.paths;

/// Convert absolute path to friendly path with ~
string friendly(string absPath)
{
    string realHome = environment["HOME"];
    if (absPath.startsWith(realHome))
    {
        return absPath.replace(realHome, "~");
    }
    return absPath;
}

/// Detect current hostname
string detectHost()
{
    string hostname = "localhost";
    try
    {
        if (exists("/etc/hostname"))
        {
            hostname = readText("/etc/hostname").strip();
        }
    }
    catch (Exception)
    {
        hostname = environment.get("HOSTNAME", "localhost");
    }
    return hostname;
}

/// Get relevant config files for selection (returns friendly paths with ~)
string[] getRelevantConfigFilesForSelection()
{
    string host = detectHost();
    string[] result;

    // Check for main config
    string mainPath = buildPath(environment["HOME"], ".owl", "main.owl");
    if (exists(mainPath))
    {
        result ~= friendly(mainPath);
    }

    // Check for host-specific config
    string hostPath = buildPath(environment["HOME"], ".owl", "hosts", host ~ ".owl");
    if (exists(hostPath))
    {
        result ~= friendly(hostPath);
    }

    // Check for common group configs (if they exist)
    string groupsDir = buildPath(environment["HOME"], ".owl", "groups");
    if (exists(groupsDir) && isDir(groupsDir))
    {
        foreach (entry; dirEntries(groupsDir, "*.owl", SpanMode.shallow))
        {
            result ~= friendly(entry.name);
        }
    }

    // If no configs found, default to main
    if (result.length == 0)
    {
        result ~= owlMainConfig();
    }

    return result;
}

/// Add package to configuration file
void addPackageToFile(string packageName, string configFile)
{
    string home = environment["HOME"];
    string configPath = configFile.replace("~", home);

    string content = "";
    if (exists(configPath))
    {
        content = readText(configPath);

        // Check if package already exists
        string line = "@package " ~ packageName;
        if (content.canFind(line))
        {
            return; // Package already exists
        }

        // Add package to existing content
        content ~= "\n@package " ~ packageName ~ "\n";
        std.file.write(configPath, content);
    }
    else
    {
        // Create new config file with template
        string templateContent = "# Owl configuration file\n"
            ~ "# This file contains package and configuration specifications for Owl\n\n"
            ~ "@packages\n\n" ~ "@package " ~ packageName ~ "\n";

        // Ensure directory exists
        string dir = dirName(configPath);
        if (!exists(dir))
        {
            mkdirRecurse(dir);
        }

        std.file.write(configPath, templateContent);
    }
}

/// Get config files for current system (absolute paths)
string[] getRelevantConfigFilesForCurrentSystem()
{
    string host = detectHost();
    string[] result;

    string mainPath = buildPath(environment["HOME"], ".owl", "main.owl");
    if (exists(mainPath))
    {
        result ~= mainPath;
    }

    string hostPath = buildPath(environment["HOME"], ".owl", "hosts", host ~ ".owl");
    if (exists(hostPath))
    {
        result ~= hostPath;
    }

    return result;
}

struct ConfigFileInfo
{
    string path;
    string configType;
    bool exists;
}

/// Resolve config file based on target string
ConfigFileInfo resolveConfigFile(string target)
{
    string owlRoot = buildPath(environment["HOME"], ".owl");

    if (target.length == 0 || target == "main" || target == "main.owl")
    {
        string mainPath = buildPath(owlRoot, "main.owl");
        return ConfigFileInfo(mainPath, "main configuration", exists(mainPath));
    }

    string cleanTarget = target.endsWith(".owl") ? target.replace(".owl", "") : target;

    // Check host configuration
    string hostPath = buildPath(owlRoot, "hosts", cleanTarget ~ ".owl");
    if (exists(hostPath))
    {
        return ConfigFileInfo(hostPath, "host configuration: " ~ cleanTarget, true);
    }

    // Check group configuration
    string groupPath = buildPath(owlRoot, "groups", cleanTarget ~ ".owl");
    if (exists(groupPath))
    {
        return ConfigFileInfo(groupPath, "group configuration: " ~ cleanTarget, true);
    }

    // Return not found
    return ConfigFileInfo("", "", false);
}
