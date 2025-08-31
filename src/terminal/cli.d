module terminal.cli;

import std.stdio : writeln;
import terminal.args;
import terminal.ui;

enum APP_VERSION = "owl 0.1.0";

// Main entrypoint
int run(string[] args)
{
    // Top-level flags
    if (hasTopFlag(args, "help"))
    {
        terminal.ui.printTopLevelHelp();
        return 0;
    }
    if (hasTopFlag(args, "version"))
    {
        writeln(APP_VERSION);
        return 0;
    }

    // Parse command/flags/args
    auto cc = parseCommandCall(args);

    // Dispatch
    return runCommand(cc);
}

// Generic dispatcher
int runCommand(const CommandCall cc)
{
    // Aliases
    string cmd = cc.command;
    if (cmd == "dr" || cmd == "dry-run")
        cmd = "dry-run";
    if (cmd == "d" || cmd == "dots")
        cmd = "dots";
    if (cmd == "up" || cmd == "upgrade")
        cmd = "upgrade";
    if (cmd == "s" || cmd == "search")
        cmd = "search";
    if (cmd == "ce" || cmd == "configedit")
        cmd = "configedit";
    if (cmd == "de" || cmd == "dotedit" || cmd == "dotsedit")
        cmd = "dotedit";
    if (cmd == "help" || cmd == "--help" || cmd == "-h")
        cmd = "help";
    if (cmd == "version" || cmd == "--version" || cmd == "-v")
        cmd = "version";

    switch (cmd)
    {
    case "apply":
        return runApply(cc);
    case "dry-run":
        return runDryRun(cc);
    case "dots":
        return runDots(cc);
    case "track":
        return runTrack(cc);
    case "hide":
        return runHide(cc);
    case "add":
        return runAdd(cc);
    case "search":
        return runSearch(cc);
    case "configedit":
        return runConfigEdit(cc);
    case "dotedit":
        return runDotEdit(cc);
    case "upgrade":
        return runUpgrade(cc);
    case "uninstall":
        return runUninstall(cc);
    case "gendb":
        return runGendb(cc);
    case "check":
        return runCheck(cc);
    case "help":
        terminal.ui.printTopLevelHelp();
        return 0;
    case "version":
        writeln(APP_VERSION);
        return 0;
    default:
        terminal.ui.printUnknownCommand(cc.command);
        terminal.ui.printTopLevelHelp();
        return 2;
    }
}

// Example handlers
import terminal.colors;
import terminal.commands;

int runCheck(const CommandCall cc)
{
    import std.process : environment;
    import std.path : buildPath;
    import config.analysis;

    string home = environment["HOME"];
    string configDir = buildPath(home, ".owl");
    string hostname = resolveHostname(cc);

    auto analysis = analyzeConfigChain(hostname, configDir);
    displayConfigAnalysis(analysis);

    return 0;
}

import config.loader;
import std.string : join, startsWith;
import std.algorithm : filter;

int runApply(const CommandCall cc)
{
    return runApplyCommand(cc);
}

int runDryRun(const CommandCall cc)
{
    CommandCall modifiedCc;
    modifiedCc.command = cc.command;
    modifiedCc.arguments = cc.arguments.dup;
    foreach (key, value; cc.flags)
    {
        modifiedCc.flags[key] = value;
    }
    modifiedCc.flags["dry-run"] = true;
    return runApplyCommand(modifiedCc);
}

int runDots(const CommandCall cc)
{
    import terminal.commands : runDotsCommand;

    return runDotsCommand(cc);
}

int runTrack(const CommandCall cc)
{
    return runTrackCommand(cc);
}

int runHide(const CommandCall cc)
{
    return runHideCommand(cc);
}

int runAdd(const CommandCall cc)
{
    return runAddCommand(cc);
}

int runConfigEdit(const CommandCall cc)
{
    return runConfigEditCommand(cc);
}

int runDotEdit(const CommandCall cc)
{
    return runDotEditCommand(cc);
}

int runUpgrade(const CommandCall cc)
{
    return runUpgradeCommand(cc);
}

int runSearch(const CommandCall cc)
{
    writeln(bold(cyan("search: Search for packages in repositories and AUR")));
    return 0;
}

int runUninstall(const CommandCall cc)
{
    writeln(bold(cyan("uninstall: Remove all managed packages and configs")));
    writeln(bold(cyan("Flags: ")), cc.flags);
    writeln(bold(cyan("Args: ")), cc.arguments);
    return 0;
}

int runGendb(const CommandCall cc)
{
    writeln(bold(cyan("gendb: Generate VCS database for development packages")));
    writeln(bold(cyan("Flags: ")), cc.flags);
    writeln(bold(cyan("Args: ")), cc.arguments);
    return 0;
}
