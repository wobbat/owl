module config.loader;

import std.file : exists, read;
import std.path : buildPath;
import std.string : indexOf, strip, startsWith;
import std.algorithm : countUntil;
import std.array : array, split;
import config.parser;

private string[] collectGroupFiles(string root, string filePath, string[string] visited)
{
    string[] acc;
    if (!exists(filePath))
        return acc;
    if (filePath in visited)
        return acc;
    visited[filePath] = "1";
    acc ~= filePath;
    auto text = cast(string) read(filePath);
    foreach (rawLine; text.split("\n"))
    {
        auto idx = rawLine.indexOf("#");
        auto line = idx >= 0 ? rawLine[0 .. idx].strip : rawLine.strip;
        if (line.startsWith("@group"))
        {
            auto name = line[6 .. $].strip;
            if (name.length > 0)
            {
                auto gp = buildPath(root, "groups", name ~ ".owl");
                acc ~= collectGroupFiles(root, gp, visited);
            }
        }
    }
    return acc;
}

public ConfigResult loadConfigChain(string root, string hostname)
{
    string[string] visited;
    string[] acc;
    auto main = buildPath(root, "main.owl");
    auto host = buildPath(root, hostname ~ ".owl");
    auto host2 = buildPath(root, "hosts", hostname ~ ".owl");
    if (exists(main))
        acc ~= collectGroupFiles(root, main, visited);
    if (exists(host))
        acc ~= collectGroupFiles(root, host, visited);
    if (exists(host2))
        acc ~= collectGroupFiles(root, host2, visited);
    ConfigResult merged;
    foreach (f; acc)
    {
        auto part = parseConfigFile(f);
        // Annotate each entry with its source file
        foreach (ref e; part.entries)
        {
            e.sourceFile = f;
            merged.entries ~= e;
        }
        foreach (k, v; part.globalEnvs)
            merged.globalEnvs[k] = v;
        merged.globalScripts ~= part.globalScripts;
    }
    return merged;
}
