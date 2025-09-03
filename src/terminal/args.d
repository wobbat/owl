module terminal.args;

import std.string : startsWith;
import std.algorithm : canFind;

struct CommandCall
{
    string command;
    bool[string] flags; // associative array for flags
    string[] arguments;
}

// Parses args into CommandCall, using defaultCommand if first arg is a flag or missing
CommandCall parseCommandCall(string[] args, string defaultCommand = "apply")
{
    CommandCall result;
    size_t argIndex = 0;
    if (args.length == 0 || args[0].startsWith("--"))
    {
        result.command = defaultCommand;
    }
    else
    {
        result.command = args[0];
        argIndex = 1;
    }
    for (; argIndex < args.length; ++argIndex)
    {
        auto arg = args[argIndex];
        if (arg.startsWith("--"))
        {
            result.flags[arg[2 .. $]] = true;
        }
        else
        {
            result.arguments ~= arg;
        }
    }
    return result;
}

// Utility for top-level flag detection
bool hasTopFlag(string[] args, string flag)
{
    return args.canFind("--" ~ flag) || args.canFind("-" ~ flag[0]);
}
