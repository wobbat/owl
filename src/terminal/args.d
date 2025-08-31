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
    CommandCall cc;
    size_t i = 0;
    if (args.length == 0 || args[0].startsWith("--"))
    {
        cc.command = defaultCommand;
    }
    else
    {
        cc.command = args[0];
        i = 1;
    }
    for (; i < args.length; ++i)
    {
        auto arg = args[i];
        if (arg.startsWith("--"))
        {
            cc.flags[arg[2 .. $]] = true;
        }
        else
        {
            cc.arguments ~= arg;
        }
    }
    return cc;
}

// Utility for top-level flag detection
bool hasTopFlag(string[] args, string flag)
{
    return args.canFind("--" ~ flag) || args.canFind("-" ~ flag[0]);
}
