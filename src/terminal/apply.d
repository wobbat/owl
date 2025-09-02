module terminal.apply;

import std.algorithm;
import std.array;
import std.format;
import std.stdio;
import std.conv;
import terminal.args;
import terminal.options;
import terminal.ui;
import terminal.colors;
import terminal.commands.apply : analyzeConfiguration, ConfigAnalysis,
    applySystemUpgrade, applyAurUpgrades, applyVcsUpgrades;
import utils.common;
import packages.packages;
import packages.pacman;
import packages.aur;
import packages.types;
import systems.dotfiles;
import systems.env;
import systems.setup;
import systems.services;
import config.loader;
import config.parser;

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
        writeln("  dotfiles: " ~ to!string(ctx.dotPkgCount));
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
    showPackageDryRun(ctx);
    showConfigDryRun(ctx);
    showServicesDryRun(ctx);
    showEnvironmentDryRun(ctx);
    return 0;
}

int executeApply(ApplyContext ctx)
{
    handlePackageManagement(ctx);
    handleConfigManagement(ctx);
    handleSetupScripts(ctx);
    handleServices(ctx);
    handleEnvironment(ctx);
    return 0;
}

void handlePackageManagement(ApplyContext ctx)
{
    sectionHeader("Pkg management", "green");

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

    updateManagedPackages(ctx.analysis.uniquePackages);
}

void handlePackageRemoval(ApplyContext ctx)
{
    writeln("Package cleanup (removing conflicting packages):");
    foreach (name; ctx.toRemove)
    {
        writeln("  " ~ errorText("remove") ~ " Removing: " ~ packageName(name));
    }
    removeUnmanagedPackages(ctx.toRemove, !ctx.options.verbose);
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
            applyAurUpgrades(ctx.options, aurOutdated);
        }
        if (ctx.options.dev)
        {
            applyVcsUpgrades(ctx.options);
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
    int codeRepo = installRepoPackages([name], true, true, progress, null);
    if (codeRepo == 0)
    {
        progress("Successfully installed " ~ name ~ " from official repositories");
        return;
    }

    if (ctx.aurAvailable && !ctx.options.noAur)
    {
        import packages.aur_build;

        if (buildAndInstallAurPackage(name, progress, ctx.options.safety, !ctx.options.unsafe))
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
    configManagementHeader();

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
        sectionHeader("Services", "teal");
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
        sectionHeader("Environment", "orange");

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
    configManagementHeader();

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
    if (ctx.analysis.allServices.length > 0)
    {
        sectionHeader("Services", "teal");
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
    if (ctx.analysis.allEnvs.length > 0 || ctx.analysis.conf.globalEnvs.length > 0)
    {
        sectionHeader("Environment", "orange");
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
