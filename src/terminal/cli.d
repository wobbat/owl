module terminal.cli;

import std.stdio : writeln;
import terminal.args;
import terminal.ui;

enum APP_VERSION = "owl 0.1.0";

// Main entrypoint
int run(string[] args)
{
    // Check for paru availability before any command execution
    import packages.pacman : ensureParuAvailable;
    ensureParuAvailable();

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
    auto commandCall = parseCommandCall(args);

    // Parse options to check if paru is requested
    import terminal.options : parseCommandOptions;
    auto opts = parseCommandOptions(commandCall.flags, commandCall.arguments);

    // Dispatch
    return runCommand(commandCall);
}

// Generic dispatcher
int runCommand(const CommandCall commandCall)
{
    // Aliases
    string command = commandCall.command;
    if (command == "dr")
        command = "dry-run";
    if (command == "d")
        command = "dots";
    if (command == "up")
        command = "upgrade";
    if (command == "ce")
        command = "configedit";
    if (command == "de")
        command = "dotedit";

    switch (command)
    {
    case "apply":
        return runApplyCommand(commandCall);
    case "dry-run":
        return runDryRun(commandCall);
    case "dots":
        return runDotsCommand(commandCall);
    case "track":
        return runTrackCommand(commandCall);
    case "hide":
        return runHideCommand(commandCall);
    case "add":
        return runAddCommand(commandCall);
    case "configedit":
        return runConfigEditCommand(commandCall);
    case "dotedit":
        return runDotEditCommand(commandCall);
    case "upgrade":
        return runUpgradeCommand(commandCall);
    case "check":
        return runCheck(commandCall);
    case "help":
        terminal.ui.printTopLevelHelp();
        return 0;
    case "version":
        writeln(APP_VERSION);
        return 0;
    default:
        terminal.ui.printUnknownCommand(commandCall.command);
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

string resolveHostname(const CommandCall commandCall)
{
    return HostDetection.detect();
}

int runCheck(const CommandCall commandCall)
{
    string home = environment["HOME"];
    string configDir = buildPath(home, ".owl");
    string hostname = resolveHostname(commandCall);

    auto analysis = analyzeConfigChain(hostname, configDir);
    displayConfigAnalysis(analysis);
    return 0;
}

int runDryRun(const CommandCall commandCall)
{
    CommandCall modifiedCommand;
    modifiedCommand.command = commandCall.command;
    modifiedCommand.arguments = commandCall.arguments.dup;
    foreach (key, value; commandCall.flags)
    {
        modifiedCommand.flags[key] = value;
    }
    modifiedCommand.flags["dry-run"] = true;
    return runApplyCommand(modifiedCommand);
}
