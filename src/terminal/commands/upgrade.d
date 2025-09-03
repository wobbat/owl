module terminal.commands.upgrade;

import terminal.commands.common_imports;

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

    // Create options structure for existing functions
    CommandOptions options;
    options.noAur = noAur;
    options.dev = dev;
    options.sync = sync;
    options.verbose = verbose;
    options.noSpinner = noSpinner;
    options.paru = true;
    if ("no-paru" in cc.flags)
        options.paru = false;

    // Upgrade all repo packages (no filtering by managed status)
    import terminal.commands.apply : applySystemUpgrade;

    applySystemUpgrade(options, false, options.paru);

    // Upgrade AUR packages if not disabled
    if (!noAur)
    {
        // Get all outdated AUR packages
        auto spinner = newSpinner("Checking AUR for updates...", !noSpinner);
        auto allOutdated = getOutdatedPackages(true, dev, null);
        auto aurOutdated = allOutdated.filter!(p => p.source == "aur").array;
        spinner.stop("Found " ~ format("%d", aurOutdated.length) ~ " AUR package(s) to upgrade");

        if (aurOutdated.length > 0)
        {
            import terminal.commands.apply : applyAurUpgrades;

            applyAurUpgrades(options, aurOutdated);
        }

        // Upgrade VCS packages if requested
        if (dev)
        {
            import terminal.commands.apply : applyVcsUpgrades;

            applyVcsUpgrades(options);
        }
    }

    import terminal.commands.apply : showAllPackagesUpgraded;

    showAllPackagesUpgraded();
    return 0;
}
