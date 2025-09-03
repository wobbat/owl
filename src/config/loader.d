module config.loader;

import std.file : exists, read;
import std.path : buildPath;
import std.string : indexOf, strip, startsWith;
import std.algorithm : countUntil;
import std.array : array, split;
import config.parser;

private string[] collectGroupFiles(string root, string filePath, string[string] visited)
{
    string[] configFiles;
    if (!exists(filePath))
        return configFiles;
    if (filePath in visited)
        return configFiles;
    visited[filePath] = "1";
    configFiles ~= filePath;
    auto text = cast(string) read(filePath);
    foreach (rawLine; text.split("\n"))
    {
        auto idx = rawLine.indexOf("#");
        auto line = idx >= 0 ? rawLine[0 .. idx].strip : rawLine.strip;
        if (line.startsWith("@group"))
        {
            auto groupName = line[6 .. $].strip;
            if (groupName.length > 0)
            {
                auto groupPath = buildPath(root, "groups", groupName ~ ".owl");
                configFiles ~= collectGroupFiles(root, groupPath, visited);
            }
        }
    }
    return configFiles;
}

public ConfigResult loadConfigChain(string root, string hostname)
{
    string[string] visited;
    string[] configFiles;
    auto mainConfigPath = buildPath(root, "main.owl");
    auto hostConfigPath = buildPath(root, hostname ~ ".owl");
    auto altHostConfigPath = buildPath(root, "hosts", hostname ~ ".owl");
    if (exists(mainConfigPath))
        configFiles ~= collectGroupFiles(root, mainConfigPath, visited);
    if (exists(hostConfigPath))
        configFiles ~= collectGroupFiles(root, hostConfigPath, visited);
    if (exists(altHostConfigPath))
        configFiles ~= collectGroupFiles(root, altHostConfigPath, visited);
    ConfigResult mergedConfig;
    foreach (configFile; configFiles)
    {
        auto partialConfig = parseConfigFile(configFile);
        // Annotate each entry with its source file
        foreach (ref entry; partialConfig.entries)
        {
            entry.sourceFile = configFile;
            mergedConfig.entries ~= entry;
        }
        foreach (key, value; partialConfig.globalEnvs)
            mergedConfig.globalEnvs[key] = value;
        mergedConfig.globalScripts ~= partialConfig.globalScripts;
    }
    return mergedConfig;
}
