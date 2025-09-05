module terminal.commands.apply;

import terminal.commands.common_imports;

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

void applySystemUpgrade(CommandOptions options, bool nothingToInstall, bool useParu = false)
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
        auto pm = initPacmanManager(false, false, false, options.paru);
        if (options.sync)
        {
            syncDatabases(true, cb, onTick, options.paru);
        }
        else
        {
            syncDatabases(false, cb, onTick, options.paru);
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

void applyAurUpgrades(CommandOptions options, OutdatedPackage[] aurPkgs, bool useParu = false)
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

            if (buildAndInstallAurPackage(p.name, progress, options.safety, false, useParu))
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

void applyVcsUpgrades(CommandOptions options, bool useParu = false)
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
        if (isVCSPackage(name))
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

            if (buildAndInstallAurPackage(name, progress, false, false, useParu))
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

struct ApplyContext
{
    bool dryRun;
    CommandOptions options;
    ConfigAnalysis analysis;
    bool aurAvailable;
    string[] toInstall;
    string[] toRemove;
    int dotPkgCount;
}

ApplyContext initializeApplyContext(bool dryRun, CommandOptions options)
{
    string host = HostDetection.detect();
    auto analysis = analyzeConfiguration(host);
    bool aurAvailable = !options.noAur;

    int dotPkgCount = 0;
    foreach (entry; analysis.conf.entries)
    {
        if (entry.configs.length > 0)
            dotPkgCount++;
    }

    // Initialize with empty arrays - will be filled in showAnalysisPhase
    return ApplyContext(dryRun, options, analysis, aurAvailable, [], [], dotPkgCount);
}

void showAnalysisPhase(ref ApplyContext ctx)
{
    sectionHeader("Analyze", "blue");

    // Show the analysis spinner here for proper order
    auto spinnerCtx = SpinnerContext(ctx.options);
    auto plan = withSpinner("Analyzing package status...", spinnerCtx, () {
        return planPackageActions(ctx.analysis.uniquePackages);
    });

    // Update the context with the calculated plan
    ctx.toInstall = plan.filter!(p => p.status == PackageActionStatus.install)
        .map!(p => p.name)
        .array;
    ctx.toRemove = plan.filter!(p => p.status == PackageActionStatus.remove)
        .map!(p => p.name)
        .array;

    if (ctx.options.debugMode)
    {
        showDebugInfo(ctx);
    }

    sectionHeader("Info", "red");
    overview(ctx.analysis.host, cast(int) ctx.analysis.uniquePackages.length);

    if (ctx.dotPkgCount > 0)
    {
        writeln(dim("  dotfiles: ") ~ to!string(ctx.dotPkgCount));
        writeln("");
    }
}

void showDebugInfo(ApplyContext ctx)
{
    sectionHeader("Debug", "magenta");
    writeln("Host used: " ~ ctx.analysis.host);
    writeln("");
    writeln("Parsed desired packages (" ~ to!string(ctx.analysis.uniquePackages.length) ~ "):");
    auto ups = ctx.analysis.uniquePackages.dup.sort;
    foreach (p; ups)
    {
        writeln("  - " ~ p);
    }
    writeln("");

    writeln("Plan: toInstall=" ~ to!string(
            ctx.toInstall.length) ~ ", toRemove=" ~ to!string(ctx.toRemove.length));
    if (ctx.toRemove.length > 0)
    {
        writeln("  Will remove (managed but not in config):");
        auto tr = ctx.toRemove.dup.sort;
        foreach (n; tr)
        {
            writeln("    - " ~ n);
        }
    }
}

int handleDryRun(ApplyContext ctx)
{
    // Dry-run: print top-level sections to mirror real run order
    sectionHeader("Pkg management", "green");
    showPackageDryRun(ctx);

    sectionHeader("Config", "magenta");
    showConfigDryRun(ctx);

    sectionHeader("Services", "teal");
    showServicesDryRun(ctx);

    sectionHeader("Environment", "orange");
    showEnvironmentDryRun(ctx);

    return 0;
}

int executeApply(ApplyContext ctx)
{
    // Top-level sections are printed here to keep flow consistent
    sectionHeader("Pkg management", "green");
    handlePackageManagement(ctx);

    sectionHeader("Config", "magenta");
    handleConfigManagement(ctx);

    if (ctx.analysis.allSetups.length > 0)
    {
        sectionHeader("Setup", "green");
        handleSetupScripts(ctx);
    }

    sectionHeader("Services", "teal");
    handleServices(ctx);

    sectionHeader("Environment", "orange");
    handleEnvironment(ctx);

    return 0;
}

void handlePackageManagement(ApplyContext ctx)
{

    auto spinnerCtx = SpinnerContext(ctx.options);
    auto allOutdated = withSpinner("Checking for package upgrades...", spinnerCtx, () {
        return getOutdatedPackages(!ctx.options.noAur, ctx.options.dev, null);
    });

    if (allOutdated.length > 0)
    {
        writeln("  Packages to upgrade:");
        foreach (pkg; allOutdated)
        {
            if (pkg.source == "aur")
            {
                writeln(terminal.colors.upgradePackageLine(pkg.name, "aur"));
            }
            else
            {
                string rep = getPackageRepository(pkg.name);
                writeln(terminal.colors.upgradePackageLine(pkg.name, rep));
            }
        }
        writeln("");
    }

    if (ctx.toInstall.length == 0 && ctx.toRemove.length == 0 && allOutdated.length == 0)
    {
        ok("There is nothing to do :)");
        writeln("");
        return;
    }

    if (ctx.toRemove.length > 0)
    {
        handlePackageRemoval(ctx);
    }

    handleUpgrades(ctx, allOutdated);

    if (ctx.toInstall.length > 0)
    {
        handlePackageInstallation(ctx);
    }

}

void handlePackageRemoval(ApplyContext ctx)
{
    writeln("Package cleanup (removing conflicting packages):");
    foreach (name; ctx.toRemove)
    {
        writeln("  " ~ errorText("remove") ~ " Removing: " ~ packageName(name));
    }
    removeUnmanagedPackages(ctx.toRemove, !ctx.options.verbose, ctx.options.paru);
    showPackagesRemoved(cast(int) ctx.toRemove.length);
}

void handleUpgrades(ApplyContext ctx, OutdatedPackage[] allOutdated)
{
    auto repoOutdated = getOutdatedPackages(false);
    if (repoOutdated.length > 0)
    {
        applySystemUpgrade(ctx.options, (ctx.toInstall.length == 0 && ctx.toRemove.length == 0));
    }

    if (ctx.aurAvailable)
    {
        auto aurOutdated = allOutdated.filter!(p => p.source == "aur").array;
        if (aurOutdated.length > 0)
        {
            applyAurUpgrades(ctx.options, aurOutdated, ctx.options.paru);
        }
        if (ctx.options.dev)
        {
            applyVcsUpgrades(ctx.options, ctx.options.paru);
        }
    }
}

void handlePackageInstallation(ApplyContext ctx)
{
    installHeader();
    foreach (name; ctx.toInstall)
    {
        auto spinnerCtx = SpinnerContext(ctx.options);
        withSpinnerAction("Installing " ~ name, spinnerCtx, (ProgressCallback progress) {
            installPackage(name, ctx, progress);
        }, "installed", "failed");
    }
}

void installPackage(string name, ApplyContext ctx, ProgressCallback progress)
{
    // Skip if paru is already installed
    if (name == "paru")
    {
        import packages.pacman : isParuInstalled;
        if (isParuInstalled())
        {
            progress("Paru is already installed");
            return;
        }
    }

    int codeRepo = installRepoPackages([name], true, true, progress, null, ctx.options.paru);
    if (codeRepo == 0)
    {
        progress("Successfully installed " ~ name ~ " from official repositories");
        return;
    }

    if (ctx.aurAvailable && !ctx.options.noAur)
    {
        import packages.aur_build;

        if (buildAndInstallAurPackage(name, progress, ctx.options.safety,
                !ctx.options.unsafe, ctx.options.paru))
        {
            progress("Successfully installed " ~ name ~ " from AUR");
        }
        else
        {
            throw new Exception("AUR build failed");
        }
    }
    else
    {
        throw new Exception("Install failed");
    }
}

void handleConfigManagement(ApplyContext ctx)
{
    // 'Config' section header is printed at the top-level; print the subheading
    writeln("  Config management:");

    if (ctx.dotPkgCount > 0)
    {
        string summary = ctx.dotPkgCount <= 5
            ? to!string(ctx.dotPkgCount) ~ " packages" : to!string(ctx.dotPkgCount) ~ " packages";
        terminal.ui.configPackagesSummary(summary);

        bool hasAnyActions = checkForAnyDotfileActions(ctx.analysis.conf.entries);

        if (!hasAnyActions)
        {
            showDotfilesUpToDate(0);
        }
        else
        {
            executeDotfileActions(ctx);
        }
    }
    else
    {
        showDotfilesUpToDate(0);
    }

    writeln("");
}

void executeDotfileActions(ApplyContext ctx)
{
    auto startTime = nowMs();
    int totalActions = 0;

    foreach (entry; ctx.analysis.conf.entries)
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
                if (action.status == "create" || action.status == "update"
                        || action.status == "conflict")
                {
                    hasPackageActions = true;
                    totalActions++;
                }
            }
            if (hasPackageActions)
            {
                showDotfileActions(entry.pkgName, actions);
            }
        }
    }

    auto duration = nowMs() - startTime;
    if (totalActions > 0)
    {
        writeln("  Dotfiles - " ~ successText("synced") ~ " " ~ dim(format("(%d actions, %dms)",
                totalActions, duration)));
    }
    else
    {
        writeln("  Dotfiles - " ~ successText("up to date") ~ " " ~ dim(format("(%dms)", duration)));
    }
}

void showDotfileActions(string pkgName, DotfileAction[] actions)
{
    writeln("  " ~ pkgName ~ " ->");
    foreach (action; actions)
    {
        if (action.status == "create")
        {
            writeln("    Copy: " ~ action.source ~ " -> " ~ action.destination);
        }
        else if (action.status == "update")
        {
            writeln("    Replace: " ~ action.source ~ " -> " ~ action.destination);
        }
        else if (action.status == "conflict")
        {
            writeln("    Conflict: " ~ action.destination ~ (action.reason.length > 0
                    ? " (" ~ action.reason ~ ")" : ""));
        }
    }
}

void handleSetupScripts(ApplyContext ctx)
{
    if (ctx.analysis.allSetups.length > 0)
    {
        runSetupScripts(ctx.analysis.allSetups);
    }
}

void handleServices(ApplyContext ctx)
{
    if (ctx.analysis.allServices.length > 0)
    {
        auto svspin = newSpinner("Validating services...",
                !ctx.options.noSpinner && !ctx.options.verbose);

        auto res = ensureServicesConfigured(ctx.analysis.allServices);
        if (res.changed)
        {
            svspin.stop("Services configured");
            writeln("");
            writeln("  " ~ symbolOk() ~ " Managed " ~ to!string(
                    ctx.analysis.allServices.length) ~ " service(s)");

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
}

void handleEnvironment(ApplyContext ctx)
{
    if (ctx.analysis.allEnvs.length > 0)
    {
        import systems.env;

        string[2][] allEnvironmentVars;
        foreach (key, value; ctx.analysis.allEnvs)
        {
            allEnvironmentVars ~= [key, value];
        }

        auto envComparison = compareEnvVars(allEnvironmentVars);
        setEnvironmentVariables(allEnvironmentVars);

        showEnvironmentChanges(envComparison);

        if (allEnvironmentVars.length > 0)
        {
            writeln("");
        }
    }
}

void showEnvironmentChanges(EnvComparison envComparison)
{
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
        writeln("  " ~ dim("(" ~ to!string(
                envComparison.unchanged.length) ~ " environment variables unchanged)"));
    }
}

void showPackageDryRun(ApplyContext ctx)
{
    if (ctx.toInstall.length > 0 || ctx.toRemove.length > 0)
    {
        installHeader();
        foreach (name; ctx.toInstall)
        {
            packageInstallProgress(name);
        }
        if (ctx.toRemove.length > 0)
        {
            writeln("Package removal simulation:");
            foreach (name; ctx.toRemove)
            {
                writeln("  " ~ errorText("remove") ~ " Would remove: " ~ packageName(name));
            }
        }
        success("Package analysis completed (dry-run mode)");
    }
}

void showConfigDryRun(ApplyContext ctx)
{
    // 'Config' section header is printed at the top-level for dry-run

    if (ctx.dotPkgCount > 0)
    {
        string summary = ctx.dotPkgCount <= 5
            ? to!string(ctx.dotPkgCount) ~ " packages" : to!string(ctx.dotPkgCount) ~ " packages";
        terminal.ui.configPackagesSummary(summary);

        auto dspin = newSpinner("    Dotfiles - checking...",
                !ctx.options.noSpinner && !ctx.options.verbose);
        dspin.stop("");

        bool hasAnyActions = checkForAnyDotfileActions(ctx.analysis.conf.entries);

        if (hasAnyActions)
        {
            showDotfileActionsPlan(ctx.analysis.conf.entries);
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
}

void showServicesDryRun(ApplyContext ctx)
{
    // 'Services' header printed at top-level for dry-run
    if (ctx.analysis.allServices.length > 0)
    {
        writeln("  Plan:");
        foreach (svc; ctx.analysis.allServices)
        {
            writeln("    ✓ Would manage " ~ packageName(svc) ~ " (system) [enable, start]");
        }
        writeln("");
        writeln("  ✓ Planned " ~ to!string(ctx.analysis.allServices.length) ~ " service(s)");
        writeln("");
    }
}

void showEnvironmentDryRun(ApplyContext ctx)
{
    // 'Environment' header printed at top-level for dry-run
    if (ctx.analysis.allEnvs.length > 0 || ctx.analysis.conf.globalEnvs.length > 0)
    {
        if (ctx.analysis.allEnvs.length > 0)
        {
            writeln("Environment variables to set:");
            foreach (k, v; ctx.analysis.allEnvs)
            {
                writeln("  ✓ Would set: " ~ packageName(k) ~ "=" ~ successText(v));
            }
            writeln("");
        }
        if (ctx.analysis.conf.globalEnvs.length > 0)
        {
            writeln("Global environment variables to set:");
            foreach (k, v; ctx.analysis.conf.globalEnvs)
            {
                writeln("  ✓ Would set global: " ~ packageName(k) ~ "=" ~ successText(v));
            }
            writeln("");
        }
    }
}

bool checkForAnyDotfileActions(ConfigEntry[] entries)
{
    foreach (entry; entries)
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
                return true;
            }
        }
    }
    return false;
}

void showDotfileActionsPlan(ConfigEntry[] entries)
{
    foreach (entry; entries)
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
                if (action.status == "create" || action.status == "update"
                        || action.status == "conflict")
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
    }
}
