module terminal.commands.apply;

import std.algorithm;
import std.array;
import std.process;
import std.stdio;
import std.string;
import std.path;
import std.file;
import std.format;
import std.conv;
import std.algorithm.sorting;

import terminal.args;
import terminal.options;
import terminal.ui;
import terminal.colors;
import terminal.prompt;
import terminal.apply;
import config.loader;
import config.parser;
import config.paths;
import utils.process;
import utils.common;
import utils.selection;
import config.write;
import packages.packages;
import packages.pacman;
import packages.aur;
import packages.types;
import systems.dotfiles;
import systems.env;
import systems.setup;
import systems.services;
import utils.sh;
import packages.state;
import packages.pkgbuild;

struct ConfigAnalysis
{
    string host;
    ConfigResult conf;
    string[] uniquePackages;
    ConfigMapping[] allConfigs;
    string[] allSetups;
    string[] allServices;
    string[string] allEnvs;
}

ConfigAnalysis analyzeConfiguration(string host)
{
    auto conf = loadConfigChain(owlConfigRoot(), host);

    // Extract unique packages (filter out special entries)
    string[] uniquePackages;
    foreach (entry; conf.entries)
    {
        if (!entry.pkgName.startsWith("__") && entry.pkgName.length > 0)
        {
            if (!uniquePackages.canFind(entry.pkgName))
            {
                uniquePackages ~= entry.pkgName;
            }
        }
    }

    // Aggregate all data from entries
    ConfigMapping[] allConfigs;
    string[] allSetups = conf.globalScripts.dup;
    string[] allServices;
    string[string] allEnvs = conf.globalEnvs.dup;

    foreach (entry; conf.entries)
    {
        allConfigs ~= entry.configs;
        allSetups ~= entry.setups;
        allServices ~= entry.services;
        foreach (key, value; entry.envs)
        {
            allEnvs[key] = value;
        }
    }

    return ConfigAnalysis(host: host, conf: conf, uniquePackages: uniquePackages, allConfigs: allConfigs,
allSetups: allSetups, allServices: allServices, allEnvs: allEnvs);
}

int runApplyCommand(const CommandCall cc)
{
    auto opts = parseCommandOptions(cc.flags, cc.arguments);
    bool dryRun = opts.dryRun || ("dry-run" in cc.flags);

    auto ctx = initializeApplyContext(dryRun, opts);
    showAnalysisPhase(ctx);

    if (dryRun)
    {
        return handleDryRun(ctx);
    }

    return executeApply(ctx);
}

void applySystemUpgrade(CommandOptions options, bool nothingToInstall)
{
    // Upgrade repo packages (no listing here; combined list is shown earlier)
    auto sp = newSpinner("Upgrading system packages...", !options.noSpinner && !options.verbose);

    // Progress callback for updating spinner text
    ProgressCallback cb = (string msg) {
        if (!options.noSpinner && !options.verbose)
        {
            sp.update(msg);
        }
    };

    // Tick callback for spinner animation
    void delegate() onTick = () {
        if (!options.noSpinner && !options.verbose)
        {
            sp.tick();
        }
    };

    try
    {
        auto pm = initPacmanManager();
        if (options.sync)
        {
            syncDatabases(true, cb, onTick);
        }
        else
        {
            syncDatabases(false, cb, onTick);
        }
        upgradeSystem(pm, options.verbose, cb, onTick);

        if (options.verbose)
        {
            showAllPackagesUpgraded();
        }
        else if (!options.noSpinner)
        {
            sp.stop("All packages upgraded!");
        }
    }
    catch (Exception)
    {
        if (!options.noSpinner && !options.verbose)
        {
            sp.fail("failed");
        }
    }
}

void applyAurUpgrades(CommandOptions options, OutdatedPackage[] aurPkgs)
{
    if (aurPkgs.length == 0)
        return;

    // Single confirmation for all AUR upgrades unless --unsafe
    if (!options.unsafe)
    {
        // Combined list is shown earlier; only prompt here
        bool proceed = options.yes;
        if (!proceed)
        {
            import terminal.prompt;

            proceed = confirmYesNo("Proceed with upgrading all AUR packages?", false);
        }
        if (!proceed)
            return;
    }

    // Perform lightweight AUR status refresh after confirmation to avoid pre-prompt delay
    try
    {
        import packages.aur_build;

        bool aurAvailable = checkAurStatus();
        if (!aurAvailable)
        {
            writeln(errorText("AUR is not available"));
            return;
        }
    }
    catch (Exception)
    {
        writeln(errorText("Could not verify AUR availability"));
        return;
    }

    foreach (p; aurPkgs)
    {
        try
        {
            auto sp = newSpinner("Upgrading AUR: " ~ p.name,
                    !options.noSpinner && !options.verbose);

            // Actually build and install the AUR package
            import packages.aur_build;

            ProgressCallback progress = (string msg) {
                if (!options.noSpinner && !options.verbose)
                    sp.update(msg);
            };

            if (buildAndInstallAurPackage(p.name, progress, options.safety, false))
            {
                if (!options.verbose)
                    sp.stop("installed");
            }
            else
            {
                if (!options.verbose)
                    sp.fail("build failed");
            }
        }
        catch (Exception)
        {
            continue;
        }
    }
}

void applyVcsUpgrades(CommandOptions options)
{
    if (options.noAur || !options.dev)
        return;

    // Get foreign packages and filter VCS (-git, -hg, etc.)
    string[2][] foreign;
    try
    {
        foreign = getForeignPackages();
    }
    catch (Exception)
    {
        foreign = [];
    }

    string[] vcsPkgs;
    foreach (pkg; foreign)
    {
        string name = pkg[0];
        if (name.endsWith("-git") || name.endsWith("-hg")
                || name.endsWith("-svn") || name.endsWith("-bzr"))
        {
            vcsPkgs ~= name;
        }
    }

    if (vcsPkgs.length == 0)
        return;

    writeln("Found " ~ to!string(vcsPkgs.length) ~ " VCS packages to check");
    foreach (name; vcsPkgs)
    {
        try
        {
            auto sp = newSpinner("Upgrading VCS: " ~ name, !options.noSpinner && !options.verbose);

            // Actually build and install the VCS package
            import packages.aur_build;

            ProgressCallback progress = (string msg) {
                if (!options.noSpinner && !options.verbose)
                    sp.update(msg);
            };

            if (buildAndInstallAurPackage(name, progress, false, false))
            {
                if (!options.verbose)
                    sp.stop("installed");
            }
            else
            {
                if (!options.verbose)
                    sp.fail("build failed");
            }
        }
        catch (Exception)
        {
            continue;
        }
    }
}

void showAllPackagesUpgraded()
{
    writeln("  " ~ symbolOk() ~ " All packages upgraded to latest versions");
}

/// Check and sync only dotfiles configurations
int dotsCheck(CommandOptions options)
{
    // Load configuration and filter entries with configs
    string host = HostDetection.detect();
    auto conf = loadConfigChain(owlConfigRoot(), host);
    ConfigEntry[] entriesWithConfigs;

    foreach (entry; conf.entries)
    {
        if (entry.configs.length > 0)
        {
            entriesWithConfigs ~= entry;
        }
    }

    if (entriesWithConfigs.length == 0)
    {
        configManagementHeader();
        showDotfilesUpToDate(0);
        return 0;
    }

    configManagementHeader();

    // Summary like legacy: names or count
    string[] pkgsWithConfigs;
    foreach (entry; entriesWithConfigs)
    {
        if (entry.pkgName.length > 0 && !entry.pkgName.startsWith("__"))
        {
            if (!pkgsWithConfigs.canFind(entry.pkgName))
            {
                pkgsWithConfigs ~= entry.pkgName;
            }
        }
    }

    if (pkgsWithConfigs.length > 0)
    {
        string summary = pkgsWithConfigs.length <= 5
            ? pkgsWithConfigs.join(", ") : to!string(pkgsWithConfigs.length) ~ " packages";
        configPackagesSummary(summary);
    }

    if (options.dryRun)
    {
        auto dspin = newSpinner("    Dotfiles - checking...",
                !options.noSpinner && !options.verbose);

        // First pass: check if ANY actions are needed across all packages
        bool hasAnyActions = false;
        foreach (entry; entriesWithConfigs)
        {
            DotfileMapping[] mappings;
            foreach (cfg; entry.configs)
            {
                mappings ~= DotfileMapping(cfg.source, cfg.dest);
            }
            if (hasActionableDotfiles(mappings))
            {
                hasAnyActions = true;
                break;
            }
        }

        dspin.stop("");

        if (!hasAnyActions)
        {
            // All packages up-to-date: show summary format like legacy
            showDotfilesUpToDate(0);
        }
        else
        {
            // Some packages need actions: show individual packages with their actions
            foreach (entry; entriesWithConfigs)
            {
                DotfileMapping[] mappings;
                foreach (cfg; entry.configs)
                {
                    mappings ~= DotfileMapping(cfg.source, cfg.dest);
                }
                auto actions = analyzeDotfiles(mappings);
                bool hadAction = false;

                foreach (action; actions)
                {
                    if (action.status == "create" || action.status == "update"
                            || action.status == "conflict")
                    {
                        if (!hadAction)
                        {
                            writeln("  " ~ entry.pkgName ~ " ->");
                            hadAction = true;
                        }

                        if (action.status == "create")
                        {
                            writeln("    Copy: " ~ action.source ~ " -> " ~ action.destination);
                        }
                        else if (action.status == "update")
                        {
                            writeln("    Replace: " ~ action.destination ~ " ← " ~ action.source);
                        }
                        else if (action.status == "conflict")
                        {
                            writeln("    Conflict: " ~ action.destination ~ (action.reason.length > 0
                                    ? " (" ~ action.reason ~ ")" : ""));
                        }
                    }
                }

                if (!hadAction)
                {
                    writeln("  " ~ entry.pkgName ~ " -> (up to date)");
                }
            }
        }
        return 0;
    }

    // First pass: check if ANY actions are needed across all packages
    bool hasAnyActions = false;
    foreach (entry; entriesWithConfigs)
    {
        DotfileMapping[] mappings;
        foreach (cfg; entry.configs)
        {
            mappings ~= DotfileMapping(cfg.source, cfg.dest);
        }
        if (hasActionableDotfiles(mappings))
        {
            hasAnyActions = true;
            break;
        }
    }

    if (!hasAnyActions)
    {
        // All packages up-to-date: show summary format like legacy
        showDotfilesUpToDate(0);
    }
    else
    {
        // Some packages need actions: show individual packages with their actions
        auto dspin = newSpinner("    Dotfiles - syncing...",
                !options.noSpinner && !options.verbose);

        foreach (entry; entriesWithConfigs)
        {
            DotfileMapping[] mappings;
            foreach (cfg; entry.configs)
            {
                mappings ~= DotfileMapping(cfg.source, cfg.dest);
            }
            auto actions = applyDotfiles(mappings);
            bool hadAction = false;

            foreach (action; actions)
            {
                if (action.status == "create" || action.status == "update"
                        || action.status == "conflict")
                {
                    if (!hadAction)
                    {
                        writeln("  " ~ entry.pkgName ~ " ->");
                        hadAction = true;
                    }

                    if (action.status == "create")
                    {
                        writeln("    Copy: " ~ action.source ~ " -> " ~ action.destination);
                    }
                    else if (action.status == "update")
                    {
                        writeln("    Replace: " ~ action.destination ~ " ← " ~ action.source);
                    }
                    else if (action.status == "conflict")
                    {
                        writeln("    Conflict: " ~ action.destination ~ (action.reason.length > 0
                                ? " (" ~ action.reason ~ ")" : ""));
                    }
                }
            }
        }
        dspin.stop("");
    }

    writeln("");
    return 0;
}
