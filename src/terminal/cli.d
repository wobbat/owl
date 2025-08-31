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
    if (cmd == "dr")
        cmd = "dry-run";
    if (cmd == "d")
        cmd = "dots";
    if (cmd == "up")
        cmd = "upgrade";
    if (cmd == "ce")
        cmd = "configedit";
    if (cmd == "de")
        cmd = "dotedit";

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
    case "configedit":
        return runConfigEdit(cc);
    case "dotedit":
        return runDotEdit(cc);
    case "upgrade":
        return runUpgrade(cc);
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

// Command implementations
import terminal.colors;
import terminal.commands.apply;
import terminal.commands.upgrade;
import terminal.commands.package_mgmt;
import terminal.commands.config_edit;
import config.analysis;
import config.loader;
import utils.common : HostDetection;
import std.process : environment;
import std.path : buildPath;

string resolveHostname(const CommandCall cc)
{
    return HostDetection.detect();
}

int runCheck(const CommandCall cc)
{
    string home = environment["HOME"];
    string configDir = buildPath(home, ".owl");
    string hostname = resolveHostname(cc);

    auto analysis = analyzeConfigChain(hostname, configDir);
    displayConfigAnalysis(analysis);
    return 0;
}

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
