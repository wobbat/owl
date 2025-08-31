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
    import std.stdio : writeln;
    import std.process : environment;
    import std.path : buildPath;

    string home = environment["HOME"];
    string configDir = buildPath(home, ".owl");
    import std.file : read;

    string hostname = "localhost";
    try
    {
        hostname = cast(string) read("/etc/hostname");
        import std.string : strip;

        hostname = hostname.strip;

    }
    catch (Exception e)
    {
    }
    if ("host" in cc.flags && cc.arguments.length > 0)
        hostname = cc.arguments[0];
    auto result = loadConfigChain(configDir, hostname);
    import std.algorithm : sum;

    size_t pkgCount, dotCount, envCount, svcCount, setupCount;
    foreach (entry; result.entries)
    {
        if (entry.pkgName.startsWith(":") || entry.pkgName.startsWith("@")
                || entry.pkgName.startsWith("!"))
            continue;
        pkgCount++;
        dotCount += entry.configs.length;
        envCount += entry.envs.length;
        svcCount += entry.services.length;
        setupCount += entry.setups.length;
    }
    envCount += result.globalEnvs.length;
    setupCount += result.globalScripts.length;
    writeln(bold(cyan("Parsed Config Chain:")));
    writeln("Packages:");
    // Print packages below
    import std.conv : to;

    string summaryLine = "Packages: " ~ to!string(pkgCount) ~ " | Dotfiles: " ~ to!string(
            dotCount) ~ " | Envs: " ~ to!string(envCount) ~ " | Services: " ~ to!string(
            svcCount) ~ " | Setups: " ~ to!string(setupCount);

    foreach (entry; result.entries)
    {
        if (entry.pkgName.startsWith(":") || entry.pkgName.startsWith("@")
                || entry.pkgName.startsWith("!"))
            continue;
        string line = bold(entry.pkgName) ~ " [" ~ entry.sourceFile ~ "]";
        if (entry.configs.length > 0)
        {
            line ~= " | configs: ";
            string[] cfgs;
            foreach (cfg; entry.configs)
                cfgs ~= cfg.source ~ "->" ~ cfg.dest;
            line ~= cfgs.join(", ");
        }
        if (entry.setups.length > 0)
        {
            line ~= " | setup: ";
            line ~= entry.setups.join(", ");
        }
        if (entry.services.length > 0)
        {
            line ~= " | service: ";
            line ~= entry.services.join(", ");
        }
        if (entry.envs.length > 0)
        {
            line ~= " | env: ";
            string[] envs;
            foreach (k, v; entry.envs)
                envs ~= k ~ "=" ~ v;
            line ~= envs.join(", ");
        }
        writeln(line);
    }
    writeln("");
    writeln(bold(cyan("Global @env:")));
    string[] envs;
    foreach (k, v; result.globalEnvs)
        envs ~= k ~ "=" ~ v;
    if (envs.length > 0)
        writeln(envs.join(" | "));
    writeln("");
    writeln(bold(cyan("Global Scripts:")));
    if (result.globalScripts.length > 0)
        writeln(result.globalScripts.join(" | "));
    writeln("");
    // Print any global services blocks (if present)
    auto globalServices = result.entries.filter!(e => e.pkgName == "__services__");
    foreach (entry; globalServices)
    {
        if (entry.services.length > 0)
        {
            writeln(bold(cyan("Global @service blocks:")));
            foreach (svc; entry.services)
            {
                writeln("  ", svc);
            }
        }
    }
    writeln("");
    writeln(bold(cyan("Summary:")));
    writeln(summaryLine);
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
    // Add dry-run flag and delegate to apply
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

int runSearch(const CommandCall cc)
{
    writeln(bold(cyan("search: Search for packages in repositories and AUR")));
    return 0;
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
