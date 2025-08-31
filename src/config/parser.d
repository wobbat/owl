module config.parser;

import std.file : read;
import std.algorithm : splitter;
import std.array : array, split, join;
import std.string : strip, startsWith, endsWith, indexOf, lastIndexOf;
import std.algorithm : countUntil;

struct ConfigMapping
{
    string source;
    string dest;
}

struct ConfigEntry
{
    string pkgName;
    ConfigMapping[] configs;
    string[] setups;
    string[] services;
    string[string] envs;
    string sourceFile;
}

struct ConfigResult
{
    ConfigEntry[] entries;
    string[string] globalEnvs;
    string[] globalScripts;
}

/// Parses a custom Owl config file
ConfigResult parseConfigFile(string path)
{
    auto content = cast(string) read(path);
    // TODO: Implement line-by-line parsing logic here
    ConfigResult result;
    struct ParserState
    {
        bool inPackages;
        bool inPkgArray;
        string pkgArrayBuf;
        string[string] globalEnvs;
        string[] globalScripts;
        ConfigEntry[string] entries;
        string currentPkg;
    }

    ParserState st;
    // D associative arrays are initialized automatically
    // st.entries is ConfigEntry[string]
    auto lines = content.split("\n");
    foreach (rawLine; lines)
    {
        auto idx = rawLine.indexOf("#");
        auto line = idx >= 0 ? rawLine[0 .. idx].strip : rawLine.strip;
        if (line.length == 0)
            continue;
        if (line.startsWith("@packages"))
        {
            st.inPackages = true;
            continue;
        }
        if (st.inPackages && line.startsWith("@"))
        {
            st.inPackages = false;
        }
        // TOML-style packages array, supports multi-line
        if (!st.inPkgArray && line.startsWith("packages") && line.countUntil("[") != -1)
        {
            auto lb = line.indexOf('[');
            if (line.countUntil("]") != -1)
            {
                auto rb = line.lastIndexOf("]");
                if (lb >= 0 && rb > lb)
                {
                    auto inside = line[(lb + 1) .. rb];
                    foreach (item; inside.splitter(','))
                    {
                        auto name = item.strip;
                        if (name.length == 0)
                            continue;
                        if ((name.startsWith('"') && name.endsWith('"'))
                                || (name.startsWith("'") && name.endsWith("'")))
                            name = name[1 .. $ - 1];
                        if (name.length > 0)
                        {
                            if (!(name in st.entries))
                                st.entries[name] = ConfigEntry(pkgName: name);
                        }
                    }
                }
                continue;
            }
            else
            {
                // start multi-line accumulation (content after '[')
                st.inPkgArray = true;
                st.pkgArrayBuf = line[(lb + 1) .. $] ~ " ";
                continue;
            }
        }
        if (st.inPkgArray)
        {
            if (line.countUntil("]") != -1)
            {
                auto rb = line.lastIndexOf("]");
                st.pkgArrayBuf ~= line[0 .. rb];
                auto inside = st.pkgArrayBuf;
                foreach (item; inside.splitter(','))
                {
                    auto name = item.strip;
                    if (name.length == 0)
                        continue;
                    if ((name.startsWith('"') && name.endsWith('"'))
                            || (name.startsWith("'") && name.endsWith("'")))
                        name = name[1 .. $ - 1];
                    if (name.length > 0)
                    {
                        if (!(name in st.entries))
                            st.entries[name] = ConfigEntry(pkgName: name);
                    }
                }
                st.inPkgArray = false;
                st.pkgArrayBuf = "";
            }
            else
            {
                st.pkgArrayBuf ~= line ~ " ";
            }
            continue;
        }
        // TOML-style section [packagename]
        if (line.startsWith("[") && line.endsWith("]") && line.countUntil("->") == -1)
        {
            auto name = line[1 .. $ - 1].strip;
            if (name.length > 0)
            {
                if (!(name in st.entries))
                    st.entries[name] = ConfigEntry(pkgName: name);
                st.currentPkg = name;
            }
            st.inPackages = false; // Ensure we exit @packages mode when entering a section
            continue;
        }
        if (st.inPackages)
        {
            auto name = line.splitter().front;
            if (name.length > 0)
            {
                if (!(name in st.entries))
                    st.entries[name] = ConfigEntry(pkgName: name);
                st.currentPkg = name; // Set currentPkg so subsequent fields attach
            }
            continue;
        }
        // Global @env
        if (line.startsWith("@env"))
        {
            auto rest = line[4 .. $].strip;
            auto eqIdx = rest.indexOf("=");
            string[] parts;
            if (eqIdx != -1)
                parts = [rest[0 .. eqIdx], rest[eqIdx + 1 .. $]];
            else
                parts = [rest];
            if (parts.length == 2)
            {
                st.globalEnvs[parts[0].strip] = parts[1].strip;
            }
            continue;
        }
        // Single package declaration
        if (line.startsWith("@package"))
        {
            auto name = line[8 .. $].strip;
            if (!(name in st.entries))
                st.entries[name] = ConfigEntry(pkgName: name);
            st.currentPkg = name;
            continue;
        }
        // Group include and @script are handled by loader; ignore here
        if (line.startsWith("@group") || line.startsWith("@script"))
        {
            continue;
        }
        // Package-scoped directives
        if (line.startsWith(":config") || line.startsWith("@config"))
        {
            auto rest = line.startsWith("@config") ? line[7 .. $].strip : line[7 .. $].strip;
            auto arrowIdx = rest.indexOf("->");
            string[] parts;
            if (arrowIdx != -1)
                parts = [rest[0 .. arrowIdx], rest[arrowIdx + 2 .. $]];
            else
                parts = rest.split();
            if (parts.length == 2)
            {
                auto src = parts[0].strip;
                auto dest = parts[1].strip;
                auto target = st.currentPkg.length > 0 ? st.currentPkg : "__configs__";
                if (!(target in st.entries))
                    st.entries[target] = ConfigEntry(pkgName: target);
                st.entries[target].configs ~= ConfigMapping(source: src, dest: dest);
            }
            continue;
        }
        if (line.startsWith(":env"))
        {
            auto rest = line[4 .. $].strip;
            auto eqIdx = rest.indexOf("=");
            string[] parts;
            if (eqIdx != -1)
                parts = [rest[0 .. eqIdx], rest[eqIdx + 1 .. $]];
            else
                parts = [rest];
            if (parts.length == 2)
            {
                auto key = parts[0].strip;
                auto val = parts[1].strip;
                auto target = st.currentPkg.length > 0 ? st.currentPkg : "__env__";
                if (!(target in st.entries))
                    st.entries[target] = ConfigEntry(pkgName: target);
                st.entries[target].envs[key] = val;
            }
            continue;
        }
        if (line.startsWith(":service"))
        {
            auto rest = line[8 .. $].strip;
            if (rest.length > 0)
            {
                auto target = st.currentPkg.length > 0 ? st.currentPkg : "__services__";
                if (!(target in st.entries))
                    st.entries[target] = ConfigEntry(pkgName: target);
                st.entries[target].services ~= rest;
            }
            continue;
        }
        if (line.startsWith("!setup") || line.startsWith(":script") || line.startsWith("@script"))
        {
            auto scriptParts = line.split(" ");
            auto script = scriptParts.length > 1 ? scriptParts[1 .. $].join(" ") : "";
            if (script.length > 0)
            {
                auto target = st.currentPkg.length > 0 ? st.currentPkg : "__scripts__";
                if (!(target in st.entries))
                    st.entries[target] = ConfigEntry(pkgName: target);
                st.entries[target].setups ~= script;
            }
            continue;
        }
    }
    // Aggregate results
    result.entries = st.entries.values.array;
    result.globalEnvs = st.globalEnvs;
    result.globalScripts = st.globalScripts;
    return result;
}
