module packages.globalenv;

import std.algorithm;
import std.array;
import std.conv;
import std.file;
import std.json;
import std.path;
import std.stdio;
import std.string;
import config.paths;

struct GlobalEnvState
{
    string schemaVersion;
    string[string] globalEnvVars;
}

/// Get path to global environment state file
string globalEnvStatePath()
{
    return owlGlobalEnvState();
}

/// Read global environment state
GlobalEnvState readGlobalEnvState()
{
    string path = globalEnvStatePath();
    GlobalEnvState state;
    state.schemaVersion = "1.0";

    if (!exists(path))
    {
        return state;
    }

    try
    {
        string content = readText(path);
        JSONValue json = parseJSON(content);

        if ("schema_version" in json)
        {
            state.schemaVersion = json["schema_version"].str;
        }

        if ("global_env_vars" in json)
        {
            foreach (key, value; json["global_env_vars"].object)
            {
                state.globalEnvVars[key] = value.str;
            }
        }

        return state;
    }
    catch (Exception e)
    {
        // If parsing fails, return default state
        return state;
    }
}

/// Write global environment state
void writeGlobalEnvState(GlobalEnvState state)
{
    string path = globalEnvStatePath();
    mkdirRecurse(dirName(path));

    JSONValue json = JSONValue();
    json["schema_version"] = JSONValue(state.schemaVersion);

    JSONValue envVars = JSONValue();
    foreach (key, value; state.globalEnvVars)
    {
        envVars[key] = JSONValue(value);
    }
    json["global_env_vars"] = envVars;

    std.file.write(path, json.toPrettyString());
}

/// Get global environment variables
string[string] readGlobalEnvVars()
{
    auto state = readGlobalEnvState();
    return state.globalEnvVars;
}

/// Set global environment variable
void setGlobalEnvVar(string key, string value)
{
    auto state = readGlobalEnvState();
    state.globalEnvVars[key] = value;
    writeGlobalEnvState(state);
}

/// Remove global environment variable
void removeGlobalEnvVar(string key)
{
    auto state = readGlobalEnvState();
    state.globalEnvVars.remove(key);
    writeGlobalEnvState(state);
}

/// Update all global environment variables
void updateGlobalEnvVars(string[string] envVars)
{
    GlobalEnvState state;
    state.schemaVersion = "1.0";
    state.globalEnvVars = envVars;
    writeGlobalEnvState(state);
}
