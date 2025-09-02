module terminal.commands.upgrade;

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

    // Upgrade all repo packages (no filtering by managed status)
    import terminal.commands.apply : applySystemUpgrade;

    applySystemUpgrade(options, false);

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
