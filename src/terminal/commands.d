module terminal.commands;

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
import config.loader;
import config.parser;
import config.paths;
import utils.process;
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

string currentHost()
{
    string hostname = "localhost";
    try
    {
        if (exists("/etc/hostname"))
        {
            hostname = readText("/etc/hostname").strip();
        }
    }
    catch (Exception)
    {
        // Try environment variable
        hostname = environment.get("HOSTNAME", "localhost");
    }
    return hostname;
}

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

    return apply(dryRun, opts);
}

int apply(bool dryRun, CommandOptions options)
{
    string host = currentHost();
    auto analysis = analyzeConfiguration(host);

    // Assume AUR available unless explicitly disabled; avoid early network check.
    bool aurAvailable = !options.noAur;

    // Analyze: compute package plan
    sectionHeader("Analyze", "blue");
    auto s = newSpinner("Analyzing package status...", !options.noSpinner);
    auto plan = planPackageActions(analysis.uniquePackages);
    s.stop("Analysis complete");

    // Debug info: show inputs used for planning
    if (options.debugMode)
    {
        sectionHeader("Debug", "magenta");
        writeln("Host used: " ~ analysis.host);
        writeln("");
        writeln("Parsed desired packages (" ~ to!string(analysis.uniquePackages.length) ~ "):");
        auto ups = analysis.uniquePackages.dup.sort;
        foreach (p; ups)
        {
            writeln("  - " ~ p);
        }
        writeln("");
    }

    string[] toInstall = plan.filter!(p => p.status == PackageActionStatus.install)
        .map!(p => p.name)
        .array;
    string[] toRemove = plan.filter!(p => p.status == PackageActionStatus.remove)
        .map!(p => p.name)
        .array;

    if (options.debugMode)
    {
        writeln("Plan: toInstall=" ~ to!string(
                toInstall.length) ~ ", toRemove=" ~ to!string(toRemove.length));
        if (toRemove.length > 0)
        {
            writeln("  Will remove (managed but not in config):");
            auto tr = toRemove.dup.sort;
            foreach (n; tr)
            {
                writeln("    - " ~ n);
            }
        }
    }

    // Info section
    sectionHeader("Info", "red");
    overview(analysis.host, cast(int) analysis.uniquePackages.length);

    // Optional: show number of dotfile packages
    int dotPkgCount = 0;
    foreach (entry; analysis.conf.entries)
    {
        if (entry.configs.length > 0)
            dotPkgCount++;
    }
    if (dotPkgCount > 0)
    {
        writeln("  dotfiles: " ~ to!string(dotPkgCount));
        writeln("");
    }

    if (dryRun)
    {
        if (toInstall.length > 0 || toRemove.length > 0)
        {
            installHeader();
            foreach (name; toInstall)
            {
                packageInstallProgress(name);
            }
            if (toRemove.length > 0)
            {
                writeln("Package removal simulation:");
                foreach (name; toRemove)
                {
                    writeln("  " ~ errorText("remove") ~ " Would remove: " ~ packageName(name));
                }
            }
            success("Package analysis completed (dry-run mode)");
        }

        // Config/dotfiles dry-run section (always show like legacy)
        configManagementHeader();

        if (dotPkgCount > 0)
        {
            string summary = dotPkgCount <= 5 ? to!string(dotPkgCount) ~ " packages" : to!string(
                    dotPkgCount) ~ " packages";
            configPackagesSummary(summary);
            auto dspin = newSpinner("    Dotfiles - checking...",
                    !options.noSpinner && !options.verbose);
            dspin.stop("");

            // Check if ANY actions are needed across all packages
            bool hasAnyActions = false;
            foreach (entry; analysis.conf.entries)
            {
                if (entry.configs.length > 0)
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
            }

            if (hasAnyActions)
            {
                // Show individual package actions when changes are needed
                foreach (entry; analysis.conf.entries)
                {
                    if (entry.configs.length > 0)
                    {
                        DotfileMapping[] mappings;
                        foreach (cfg; entry.configs)
                        {
                            mappings ~= DotfileMapping(cfg.source, cfg.dest);
                        }
                        auto actions = analyzeDotfiles(mappings);
                        bool hasPackageActions = false;
                        foreach (action; actions)
                        {
                            if (action.status == "create"
                                    || action.status == "update" || action.status == "conflict")
                            {
                                hasPackageActions = true;
                                break;
                            }
                        }
                        if (hasPackageActions)
                        {
                            writeln("  " ~ entry.pkgName ~ " ->");
                            foreach (action; actions)
                            {
                                if (action.status == "create")
                                {
                                    writeln("    Copy: " ~ action.source
                                            ~ " -> " ~ action.destination);
                                }
                                else if (action.status == "update")
                                {
                                    writeln(
                                            "    Replace: " ~ action.destination
                                            ~ " ← " ~ action.source);
                                }
                                else if (action.status == "conflict")
                                {
                                    writeln("    Conflict: " ~ action.destination ~ (action.reason.length > 0
                                            ? " (" ~ action.reason ~ ")" : ""));
                                }
                            }
                        }
                    }
                }
            }
            else
            {
                showDotfilesUpToDate(0);
            }
        }
        else
        {
            showDotfilesUpToDate(0);
        }

        writeln("");

        // Services dry-run
        if (analysis.allServices.length > 0)
        {
            sectionHeader("Services", "teal");
            writeln("  Plan:");
            foreach (svc; analysis.allServices)
            {
                writeln("    ✓ Would manage " ~ packageName(svc) ~ " (system) [enable, start]");
            }
            writeln("");
            writeln("  ✓ Planned " ~ to!string(analysis.allServices.length) ~ " service(s)");
            writeln("");
        }

        // Env dry-run
        if (analysis.allEnvs.length > 0 || analysis.conf.globalEnvs.length > 0)
        {
            sectionHeader("Environment", "blue");
            if (analysis.allEnvs.length > 0)
            {
                writeln("Environment variables to set:");
                foreach (k, v; analysis.allEnvs)
                {
                    writeln("  ✓ Would set: " ~ packageName(k) ~ "=" ~ successText(v));
                }
                writeln("");
            }
            if (analysis.conf.globalEnvs.length > 0)
            {
                writeln("Global environment variables to set:");
                foreach (k, v; analysis.conf.globalEnvs)
                {
                    writeln("  ✓ Would set global: " ~ packageName(k) ~ "=" ~ successText(v));
                }
                writeln("");
            }
        }
        return 0;
    }

    // Real apply path
    // Upgrade system packages (repos first, then AUR updates)
    // Pkg management unified section (remove, upgrade, install)
    sectionHeader("Pkg management", "yellow");

    // Show combined upgrade plan (repo + AUR) once
    auto upgradeSpinner = newSpinner("Checking for package upgrades...", !options.noSpinner);
    ProgressCallback upgradeProgress = (string msg) {
        if (!options.noSpinner)
            upgradeSpinner.update(msg);
        else
            writeln(msg);
    };
    auto allOutdated = getOutdatedPackages(!options.noAur, options.dev, upgradeProgress);
    upgradeSpinner.stop("Package upgrade check complete");
    if (allOutdated.length > 0)
    {
        writeln("  Packages to upgrade:");
        foreach (pkg; allOutdated)
        {
            if (pkg.source == "aur")
            {
                writeln(upgradePackageLine(pkg.name, "aur"));
            }
            else
            {
                string rep = getPackageRepository(pkg.name);
                writeln(upgradePackageLine(pkg.name, rep));
            }
        }
        writeln("");
    }

    // If there is truly nothing to do in package mgmt, say so once
    if (toInstall.length == 0 && toRemove.length == 0 && allOutdated.length == 0)
    {
        ok("There is nothing to do :)");
        writeln("");
    }

    // Remove unmanaged packages first (if any)
    if (toRemove.length > 0)
    {
        writeln("Package cleanup (removing conflicting packages):");
        foreach (name; toRemove)
        {
            writeln("  " ~ errorText("remove") ~ " Removing: " ~ packageName(name));
        }
        removeUnmanagedPackages(toRemove, !options.verbose);
        showPackagesRemoved(cast(int) toRemove.length);
    }

    auto repoOutdated = getOutdatedPackages(false);
    if (repoOutdated.length > 0)
    {
        applySystemUpgrade(options, (toInstall.length == 0 && toRemove.length == 0));
    }

    if (aurAvailable)
    {
        auto aurOutdated = allOutdated.filter!(p => p.source == "aur").array;
        if (aurOutdated.length > 0)
        {
            applyAurUpgrades(options, aurOutdated);
        }
        if (options.dev)
        {
            applyVcsUpgrades(options);
        }
    }

    // Install new packages (if any)
    if (toInstall.length > 0)
    {
        installHeader();
        foreach (name; toInstall)
        {
            auto sp = newSpinner("Installing " ~ name, !options.noSpinner && !options.verbose);

            // Progress callback for updating spinner text
            ProgressCallback progress = (string msg) {
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

            // Try repo install first
            int codeRepo = installRepoPackages([name], true, true, progress, onTick);
            if (codeRepo == 0)
            {
                sp.update("Successfully installed " ~ name ~ " from official repositories");
                if (!options.verbose)
                    sp.stop("installed");
                continue;
            }

            // Fallback to AUR if allowed
            if (aurAvailable && !options.noAur)
            {
                try
                {
                    // Build and install AUR package
                    import packages.aur_build;

                    ProgressCallback aurProgress = (string msg) {
                        if (!options.noSpinner && !options.verbose)
                        {
                            sp.update(msg);
                        }
                    };

                    if (buildAndInstallAurPackage(name, aurProgress,
                            options.safety, !options.unsafe))
                    {
                        sp.update("Successfully installed " ~ name ~ " from AUR");
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
                    if (!options.verbose)
                        sp.fail("failed");
                }
            }
            else
            {
                if (!options.verbose)
                    sp.fail("install failed");
            }
        }
    }

    // Update managed package tracking
    updateManagedPackages(analysis.uniquePackages);

    // Always show Config section like legacy, even when no dotfiles exist
    configManagementHeader();

    if (dotPkgCount > 0)
    {
        string summary = dotPkgCount <= 5 ? to!string(dotPkgCount) ~ " packages" : to!string(
                dotPkgCount) ~ " packages";
        configPackagesSummary(summary);

        // First pass: check if ANY actions are needed across all packages
        bool hasAnyActions = false;
        foreach (entry; analysis.conf.entries)
        {
            if (entry.configs.length > 0)
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
        }

        if (!hasAnyActions)
        {
            // All packages up-to-date: show summary format like legacy  
            int dur = 0; // No spinner needed for immediate check
            showDotfilesUpToDate(dur);
        }
        else
        {
            // Execute dotfile operations and show results
            auto startTime = nowMs();
            int totalActions = 0;

            foreach (entry; analysis.conf.entries)
            {
                if (entry.configs.length > 0)
                {
                    DotfileMapping[] mappings;
                    foreach (cfg; entry.configs)
                    {
                        mappings ~= DotfileMapping(cfg.source, cfg.dest);
                    }
                    auto actions = applyDotfiles(mappings);
                    bool hasPackageActions = false;
                    foreach (action; actions)
                    {
                        if (action.status == "create"
                                || action.status == "update" || action.status == "conflict")
                        {
                            hasPackageActions = true;
                            totalActions++;
                        }
                    }
                    if (hasPackageActions)
                    {
                        writeln("  " ~ entry.pkgName ~ " ->");
                        foreach (action; actions)
                        {
                            if (action.status == "create")
                            {
                                writeln("    Copy: " ~ action.source ~ " -> " ~ action.destination);
                            }
                            else if (action.status == "update")
                            {
                                writeln("    Replace: " ~ action.source ~ " -> "
                                        ~ action.destination);
                            }
                            else if (action.status == "conflict")
                            {
                                writeln("    Conflict: " ~ action.destination ~ (action.reason.length > 0
                                        ? " (" ~ action.reason ~ ")" : ""));
                            }
                        }
                    }
                }
            }

            // Show completion summary
            auto duration = nowMs() - startTime;
            if (totalActions > 0)
            {
                writeln("  Dotfiles - " ~ successText("synced") ~ " " ~ dim(format("(%d actions, %dms)",
                        totalActions, duration)));
            }
            else
            {
                writeln("  Dotfiles - " ~ successText("up to date") ~ " " ~ dim(format("(%dms)",
                        duration)));
            }
        }
    }
    else
    {
        // No dotfiles to manage, show up to date message anyway
        showDotfilesUpToDate(0);
    }

    writeln("");

    // Run setup scripts (global first as constructed above)
    if (analysis.allSetups.length > 0)
    {
        runSetupScripts(analysis.allSetups);
    }

    // Services
    if (analysis.allServices.length > 0)
    {
        sectionHeader("Services", "teal");
        auto svspin = newSpinner("Validating services...", !options.noSpinner && !options.verbose);

        auto res = ensureServicesConfigured(analysis.allServices);
        if (res.changed)
        {
            svspin.stop("Services configured");
            writeln("");
            writeln("  " ~ symbolOk() ~ " Managed " ~ to!string(
                    analysis.allServices.length) ~ " service(s)");

            if (res.enabledServices.length > 0)
            {
                writeln("    Enabled: " ~ res.enabledServices.join(", "));
            }
            if (res.startedServices.length > 0)
            {
                writeln("    Started: " ~ res.startedServices.join(", "));
            }
            if (res.failedServices.length > 0)
            {
                writeln("    " ~ errorText("Failed: " ~ res.failedServices.join(", ")));
            }
            writeln("");
        }
        else
        {
            svspin.stop("Service state verified");
            writeln("");
        }
    }

    // Environment variables
    if (analysis.allEnvs.length > 0)
    {
        sectionHeader("Environment", "blue");

        // Convert from associative array to array of pairs
        import systems.env;

        string[2][] allEnvironmentVars;
        foreach (key, value; analysis.allEnvs)
        {
            allEnvironmentVars ~= [key, value];
        }

        auto envComparison = compareEnvVars(allEnvironmentVars);
        setEnvironmentVariables(allEnvironmentVars);

        // Display environment variable changes intelligently
        if (envComparison.added.length > 0)
        {
            foreach (env; envComparison.added)
            {
                ok("Set: " ~ packageName(env[0]) ~ "=" ~ successText(env[1]));
            }
        }

        if (envComparison.updated.length > 0)
        {
            foreach (env; envComparison.updated)
            {
                ok("Updated: " ~ packageName(env[0]) ~ "=" ~ successText(env[1]));
            }
        }

        if (envComparison.removed.length > 0)
        {
            foreach (key; envComparison.removed)
            {
                ok("Removed: " ~ packageName(key));
            }
        }

        if (envComparison.unchanged.length > 0 && envComparison.added.length == 0
                && envComparison.updated.length == 0 && envComparison.removed.length == 0)
        {
            ok("Environment variables maintained (" ~ to!string(
                    envComparison.unchanged.length) ~ " unchanged)");
        }
        else if (envComparison.unchanged.length > 0)
        {
            import terminal.colors : dim;

            writeln("  " ~ dim("(" ~ to!string(
                    envComparison.unchanged.length) ~ " environment variables unchanged)"));
        }

        if (allEnvironmentVars.length > 0)
        {
            writeln("");
        }
    }

    return 0;
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

string upgradePackageLine(string name, string repo)
{
    return "    " ~ warningText("upgrade") ~ " " ~ packageName(name) ~ " (" ~ repo ~ ")";
}

void showAllPackagesUpgraded()
{
    writeln("  " ~ symbolOk() ~ " All packages upgraded to latest versions");
}

/// Run upgrade command - upgrade all packages to latest versions
int runUpgradeCommand(const CommandCall cc)
{
    import packages.packages;

    bool noAur = ("no-aur" in cc.flags) !is null;
    bool dev = ("dev" in cc.flags) !is null;
    bool sync = ("sync" in cc.flags) !is null || ("refresh" in cc.flags) !is null;
    bool verbose = ("verbose" in cc.flags) !is null;
    bool noSpinner = ("no-spinner" in cc.flags) !is null;

    sectionHeader("Upgrade", "yellow");

    // Get all outdated packages 
    auto spinner = newSpinner("Checking for updates...", !noSpinner);
    auto allOutdated = getOutdatedPackages(!noAur, dev, null);
    spinner.stop("Found " ~ format("%d", allOutdated.length) ~ " package(s) to upgrade");

    if (allOutdated.length == 0)
    {
        ok("All packages are up to date");
        return 0;
    }

    // Show upgrade plan
    writeln("Packages to upgrade:");
    foreach (pkg; allOutdated)
    {
        if (pkg.source == "aur")
        {
            writeln(upgradePackageLine(pkg.name, "aur"));
        }
        else
        {
            string repo = getPackageRepository(pkg.name);
            writeln(upgradePackageLine(pkg.name, repo));
        }
    }
    writeln("");

    // Separate repo and AUR packages
    auto repoOutdated = allOutdated.filter!(p => p.source != "aur").array;
    auto aurOutdated = allOutdated.filter!(p => p.source == "aur").array;

    bool success = true;

    // Create options structure for existing functions
    CommandOptions options;
    options.noAur = noAur;
    options.dev = dev;
    options.sync = sync;
    options.verbose = verbose;
    options.noSpinner = noSpinner;

    // Upgrade repo packages first
    if (repoOutdated.length > 0)
    {
        applySystemUpgrade(options, false);
    }

    // Upgrade AUR packages
    if (!noAur && aurOutdated.length > 0)
    {
        applyAurUpgrades(options, aurOutdated);
    }

    // Upgrade VCS packages if requested
    if (!noAur && dev)
    {
        applyVcsUpgrades(options);
    }

    showAllPackagesUpgraded();
    return 0;
}

void configPackagesSummary(string summary)
{
    writeln("  " ~ summary);
}

int runAddCommand(const CommandCall cc)
{
    auto opts = parseCommandOptions(cc.flags, cc.arguments);
    return addPackage(cc.arguments, opts);
}

int addPackage(const string[] searchTerms, CommandOptions options)
{
    if (searchTerms.length == 0)
    {
        errorOutput("Please provide search terms");
        return 1;
    }

    auto results = searchAny(cast(string[]) searchTerms, options.source);

    // Display results exactly like nim version
    writeln("\n" ~ bold("Found") ~ " " ~ format("%d", results.length) ~ " package(s):\n");

    if (results.length == 0)
    {
        writeln("");
        return 1;
    }

    // Use countdown numbering (most relevant at bottom)
    foreach (ulong num; 1 .. results.length + 1)
    {
        ulong idx = results.length - num;
        auto result = results[idx];

        string numStr = numberBrackets(cast(int) num);
        string name = highlight(result.name);
        string versionStr = successText(result.ver);
        string tag = result.source == PackageSource.aur ? brackets("aur",
                Warning) : brackets(result.repo, Repository);
        string status = result.installed ? " " ~ successText("installed") : "";
        string desc = result.description.length > 0 ? " - " ~ description(result.description) : "";

        writeln(numStr ~ " " ~ name ~ " " ~ versionStr ~ " " ~ tag ~ status ~ desc);
    }
    writeln("");

    if (options.dryRun)
    {
        infoOutput("Dry run mode - would prompt for selection");
        return 0;
    }

    // Interactive selection
    int selNum = promptSelection(cast(int) results.length);
    if (selNum <= 0 || selNum > results.length)
    {
        writeln(red("✗ " ~ "Invalid selection"));
        return 1;
    }

    // Fix selection mapping: reverse the index to match display order
    auto chosen = results[results.length - selNum];
    return addPackageToConfig(chosen, options);
}

int addPackageToConfig(SearchResult sel, CommandOptions options)
{
    sectionHeader("Add Package to Configuration", "blue");

    string targetFile = options.file;

    if (targetFile.length == 0)
    {
        auto files = getRelevantConfigFilesForSelection();
        if (files.length == 0)
        {
            targetFile = owlMainConfig();
        }
        else if (files.length == 1)
        {
            targetFile = files[0];
        }
        else
        {
            writeln("\n" ~ bold("Select a configuration file:") ~ "\n");

            // Show countdown numbering (most relevant at bottom)
            foreach (ulong num; 1 .. files.length + 1)
            {
                ulong idx = files.length - num;
                string file = files[idx];
                string friendly = file.replace("~/", "");
                string numberPart = numberBrackets(cast(int) num);
                string fileName = packageName(friendly.canFind('/')
                        ? friendly.split('/')[$ - 1] : friendly);
                string pathPart = "(" ~ highlight(friendly) ~ ")";

                // Count packages in this file (simplified for now)
                string countPart = brackets("config", Repository);
                writeln(numberPart ~ " " ~ fileName ~ " " ~ pathPart ~ " " ~ countPart);
            }
            writeln("");

            int pick = promptSelection(cast(int) files.length);
            if (pick <= 0 || pick > files.length)
            {
                errorOutput("Invalid selection");
                return 1;
            }
            // Fix selection mapping: reverse the index to match display order
            targetFile = files[files.length - pick];
        }
    }

    if (options.dryRun)
    {
        infoOutput(format("Would add '%s' to %s", sel.name, targetFile));
        return 0;
    }

    // Actually add the package
    addPackageToFile(sel.name, targetFile);
    successOutput(format("Added '%s' to %s", sel.name, targetFile));

    return 0;
}

int runConfigEditCommand(const CommandCall cc)
{
    string target = cc.arguments.length > 0 ? cc.arguments[0] : "";
    return configEdit(target);
}

int configEdit(string target)
{
    if (!isEditorAvailable())
    {
        errorOutput("EDITOR environment variable is not set");
        return 1;
    }

    auto configInfo = resolveConfigFile(target);
    if (!configInfo.exists)
    {
        string errorMsg = target.length > 0 ? format("No configuration found for '%s' (checked main, hosts, and groups)",
                target) : "No configuration found for 'main' (checked main, hosts, and groups)";
        errorOutput(errorMsg);
        return 1;
    }

    infoOutput(format("Found %s", configInfo.configType));
    infoOutput(format("Opening %s with %s", configInfo.path, getEditorBinary()));

    auto result = runEditor(getEditorBinary(), configInfo.path);
    if (!result.success())
    {
        errorOutput(result.errorMessage);
        return 1;
    }
    return result.exitCode;
}

int runDotEditCommand(const CommandCall cc)
{
    string target = cc.arguments.length > 0 ? cc.arguments[0] : "";
    return dotEdit(target);
}

/// Find dotfile path based on target string
struct DotfilePath
{
    string path;
    string foundType;
}

DotfilePath findDotfilePath(string target)
{
    string dotfilesDir = owlDotfilesDir();

    if (target.length == 0)
    {
        return DotfilePath(dotfilesDir, "dotfiles directory");
    }

    // Load config to check mappings
    string host = currentHost();
    auto conf = loadConfigChain(owlConfigRoot(), host);

    // Check config mappings first
    foreach (entry; conf.entries)
    {
        foreach (mapping; entry.configs)
        {
            string absSrc = mapping.source;
            if (!absSrc.startsWith("/") && !absSrc.startsWith("./") && !absSrc.startsWith("../"))
            {
                absSrc = buildPath(dotfilesDir, absSrc);
            }

            string base = baseName(absSrc);
            bool matches = (absSrc.endsWith(target) || base == target
                    || absSrc.endsWith("/" ~ target) || mapping.source == target
                    || mapping.source.endsWith("/" ~ target));

            if (matches)
            {
                return DotfilePath(absSrc, "config mapping");
            }
        }
    }

    // Check direct file path
    string directPath = buildPath(dotfilesDir, target);
    if (exists(directPath) && isFile(directPath))
    {
        return DotfilePath(directPath, "direct path");
    }

    // Check directory path
    if (exists(directPath) && isDir(directPath))
    {
        return DotfilePath(directPath, "directory");
    }

    // Default to dotfiles directory
    return DotfilePath(dotfilesDir, "dotfiles directory");
}

int dotEdit(string target)
{
    if (!isEditorAvailable())
    {
        errorOutput("EDITOR environment variable is not set");
        return 1;
    }

    string dotfilesDir = owlDotfilesDir();
    auto dotfilePath = findDotfilePath(target);

    // Ensure dotfiles directory exists
    if (!exists(dotfilesDir))
    {
        try
        {
            mkdirRecurse(dotfilesDir);
            infoOutput(format("Created dotfiles directory: %s", dotfilesDir));
        }
        catch (Exception e)
        {
            errorOutput(format("Failed to create dotfiles directory: %s", e.msg));
            return 1;
        }
    }

    // Show what we found
    if (target.length > 0 && dotfilePath.foundType == "dotfiles directory")
    {
        infoOutput(format("Dotfile '%s' not found in configuration, opening dotfiles directory",
                target));
    }
    else
    {
        infoOutput(format("Found %s: %s", dotfilePath.foundType, dotfilePath.path));
    }

    // Open with editor
    string finalPath = (exists(dotfilePath.path)) ? dotfilePath.path : dotfilesDir;
    infoOutput(format("Opening %s with %s", finalPath, getEditorBinary()));

    auto result = runEditor(getEditorBinary(), finalPath);
    if (!result.success())
    {
        errorOutput(result.errorMessage);
        return 1;
    }
    return result.exitCode;
}

/// Run track command - track explicitly-installed packages into Owl configs
int runTrackCommand(const CommandCall cc)
{
    auto opts = parseCommandOptions(cc.flags, cc.arguments);
    return trackPackages(cc.arguments, opts);
}

/// Track explicitly-installed packages into configuration
int trackPackages(const string[] args, CommandOptions options)
{
    string host = currentHost();
    auto candidates = computeTrackCandidates(host);

    if (candidates.length == 0)
    {
        ok("No untracked explicit packages found");
        return 0;
    }

    writeln("\n" ~ bold("Found") ~ " " ~ to!string(candidates.length) ~ " untracked package(s):\n");

    // Display packages in countdown order (most relevant at bottom)
    foreach (ulong i; 1 .. candidates.length + 1)
    {
        ulong idx = candidates.length - i;
        string pkg = candidates[idx];
        string numberPart = successText("[") ~ to!string(i) ~ successText("]");
        writeln(numberPart ~ " " ~ packageName(pkg));
    }
    writeln("");

    int selection = promptSelection(cast(int) candidates.length);
    if (selection <= 0 || selection > candidates.length)
    {
        return 0;
    }

    // Fix selection mapping: reverse the index to match display order
    string selected = candidates[candidates.length - selection];

    // Select config file
    auto files = getRelevantConfigFilesForCurrentSystem();
    string targetFile = "";

    if (files.length == 0)
    {
        targetFile = owlMainConfig();
    }
    else if (files.length == 1)
    {
        string homeDir = environment["HOME"];
        targetFile = files[0].replace(homeDir, "~");
    }
    else
    {
        writeln("\n" ~ bold("Select a configuration file:") ~ "\n");

        // Display files in countdown order (most relevant at bottom)
        foreach (ulong i; 1 .. files.length + 1)
        {
            ulong idx = files.length - i;
            string file = files[idx];
            string friendly = file.replace(environment["HOME"], "~");
            string numberPart = successText("[") ~ to!string(i) ~ successText("]");
            string fileName = packageName(friendly.split('/')[$ - 1]);
            string pathPart = "(" ~ friendly ~ ")";
            writeln(numberPart ~ " " ~ fileName ~ " " ~ pathPart);
        }
        writeln("");

        int pick = promptSelection(cast(int) files.length);
        if (pick <= 0 || pick > files.length)
        {
            errorOutput("Invalid selection");
            return 1;
        }
        string homeDir = environment["HOME"];
        // Fix selection mapping: reverse the index to match display order
        targetFile = files[files.length - pick].replace(homeDir, "~");
    }

    // Add package to config file
    addPackageToFile(selected, targetFile);
    import terminal.ui;

    success("Tracked '" ~ selected ~ "' in " ~ targetFile);
    return 0;
}

/// Run hide command - hide packages from track suggestions
int runHideCommand(const CommandCall cc)
{
    auto opts = parseCommandOptions(cc.flags, cc.arguments);
    return hidePackages(cc.arguments, opts, cc.flags);
}

/// Hide packages from track suggestions with flag support
int hidePackages(const string[] args, CommandOptions options, const bool[string] flags)
{
    // Check for show hidden flag
    bool hasShowHidden = ("show-hidden" in flags) || ("show" in flags);

    // Check for remove flag
    string removeArg = "";
    if ("remove" in flags && args.length > 0)
    {
        removeArg = args[0];
    }

    // Handle --show-hidden flag
    if (hasShowHidden)
    {
        sectionHeader("Hidden (Untracked) Packages", "blue");
        auto hidden = readUntracked();
        if (hidden.length == 0)
        {
            ok("No hidden packages");
            return 0;
        }
        foreach (name; hidden)
        {
            writeln(name);
        }
        return 0;
    }

    // Handle --remove flag
    if (removeArg.length > 0)
    {
        sectionHeader("Update Hidden List", "blue");
        auto hidden = readUntracked();
        if (hidden.canFind(removeArg))
        {
            removeFromUntracked(removeArg);
            import terminal.ui;

            success("Removed '" ~ removeArg ~ "' from hidden list");
        }
        else
        {
            import terminal.ui : error;

            error("'" ~ removeArg ~ "' not found in hidden list");
        }
        return 0;
    }

    // Normal hide functionality
    sectionHeader("Hide Packages", "blue");
    string host = currentHost();
    auto candidates = computeTrackCandidates(host);

    if (candidates.length == 0)
    {
        ok("No candidates to hide");
        return 0;
    }

    writeln("\n" ~ bold("Candidate packages (hide to ignore in track):") ~ "\n");

    // Display candidates in countdown order (most relevant at bottom)
    foreach (ulong i; 1 .. candidates.length + 1)
    {
        ulong idx = candidates.length - i;
        string pkg = candidates[idx];
        string numberPart = successText("[") ~ to!string(i) ~ successText("]");
        writeln(numberPart ~ " " ~ packageName(pkg));
    }
    writeln("");

    int selection = promptSelection(cast(int) candidates.length);
    if (selection <= 0 || selection > candidates.length)
    {
        return 0;
    }

    // Fix selection mapping: reverse the index to match display order
    string selected = candidates[candidates.length - selection];
    addToUntracked(selected);
    import terminal.ui : success;

    success("Hidden '" ~ selected ~ "' from track suggestions");
    return 0;
}

/// Run dots command - check and sync only dotfiles configurations
int runDotsCommand(const CommandCall cc)
{
    auto opts = parseCommandOptions(cc.flags, cc.arguments);
    return dotsCheck(opts);
}

/// Check and sync only dotfiles configurations
int dotsCheck(CommandOptions options)
{
    // Load configuration and filter entries with configs
    string host = currentHost();
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
