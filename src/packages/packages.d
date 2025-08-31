module packages.packages;

import std.algorithm;
import std.stdio;
import std.array;
import std.container : redBlackTree;
import std.conv : to;
import core.stdc.stdlib : exit;
import packages.types;
import packages.pacman;
import packages.aur;

/// Check if a package is a VCS package (ends with -git, -hg, etc.)
bool isVCSPackage(string name)
{
    return name.endsWith("-git") || name.endsWith("-hg")
        || name.endsWith("-svn") || name.endsWith("-bzr");
}

import packages.aur_build;

PackageAction[] planPackageActions(string[] desired)
{
    // Compare desired with installed to compute installs; only remove previously-managed packages
    auto installed = getInstalledPackages().map!(p => p[0]).array.redBlackTree;
    auto desiredSet = desired.redBlackTree;

    PackageAction[] actions;

    foreach (name; desired)
    {
        if (name !in installed)
        {
            actions ~= PackageAction(name: name, status: PackageActionStatus.install);
        }
    }

    // TODO: Implement managed packages tracking
    // Only propose removals for packages we previously managed
    // For now, we'll skip removals until state management is implemented

    return actions;
}

void removeUnmanagedPackages(string[] names, bool quiet = true)
{
    removePackages(names, recursive: true, assumeYes: quiet);
}

void updateManagedPackages(string[] names)
{
    // TODO: Persist the set of managed packages after a successful apply
    // This will require implementing state management
}

/// Apply system package upgrades (repo packages)
bool applySystemUpgrade(bool sync = false, ProgressCallback progress = null)
{
    try
    {
        if (sync)
        {
            if (progress)
                progress("Syncing package databases...");
            syncDatabases(progress);
        }

        if (progress)
            progress("Upgrading system packages...");

        return upgradeSystem(progress);
    }
    catch (Exception e)
    {
        return false;
    }
}

/// Apply AUR package upgrades
bool applyAurUpgrades(OutdatedPackage[] aurPkgs, ProgressCallback progress = null)
{
    if (aurPkgs.length == 0)
        return true;

    bool allSuccessful = true;

    foreach (pkg; aurPkgs)
    {
        try
        {
            if (progress)
                progress("Upgrading AUR package: " ~ pkg.name);

            if (!buildAndInstallAurPackage(pkg.name, progress))
            {
                allSuccessful = false;
            }
        }
        catch (Exception e)
        {
            allSuccessful = false;
        }
    }

    return allSuccessful;
}

/// Apply VCS package upgrades (git packages, etc.)
bool applyVcsUpgrades(ProgressCallback progress = null)
{
    try
    {
        string[2][] foreignPkgs = getForeignPackages();
        string[] vcsPkgs;

        foreach (pkg; foreignPkgs)
        {
            string name = pkg[0];
            if (name.endsWith("-git") || name.endsWith("-hg")
                    || name.endsWith("-svn") || name.endsWith("-bzr"))
            {
                vcsPkgs ~= name;
            }
        }

        if (vcsPkgs.length == 0)
            return true;

        bool allSuccessful = true;

        foreach (pkg; vcsPkgs)
        {
            try
            {
                if (progress)
                    progress("Upgrading VCS package: " ~ pkg);

                if (!buildAndInstallAurPackage(pkg, progress))
                {
                    allSuccessful = false;
                }
            }
            catch (Exception e)
            {
                allSuccessful = false;
            }
        }

        return allSuccessful;
    }
    catch (Exception e)
    {
        return false;
    }
}

OutdatedPackage[] getOutdatedPackages(bool includeAur = true,
        bool includeDev = false, ProgressCallback progress = null)
{
    OutdatedPackage[] result = getOutdatedRepoPackages();

    if (includeAur)
    {
        // Augment by comparing foreign packages against AUR info
        auto foreignPkgs = getForeignPackages();

        // Filter packages and collect names for batch query
        string[] aurPackageNames;
        string[string] packageVersions; // name -> currentVersion

        foreach (pkg; foreignPkgs)
        {
            string name = pkg[0];
            string currentVersion = pkg[1];

            // Skip VCS packages unless dev mode is enabled
            if (isVCSPackage(name) && !includeDev)
                continue;

            aurPackageNames ~= name;
            packageVersions[name] = currentVersion;
        }

        if (aurPackageNames.length > 0)
        {
            if (progress)
                progress(
                        "Checking " ~ aurPackageNames.length.to!string
                        ~ " AUR packages in batch...");

            try
            {
                // Make single batch API call
                auto aurInfos = infoBatch(aurPackageNames);

                // Process results
                foreach (aurInfo; aurInfos)
                {
                    if (aurInfo.name.length > 0 && aurInfo.ver.length > 0
                            && aurInfo.name in packageVersions
                            && aurInfo.ver != packageVersions[aurInfo.name])
                    {
                        result ~= OutdatedPackage(name: aurInfo.name, fromVersion: packageVersions[aurInfo
                                .name], toVersion: aurInfo.ver, source: "aur");
                    }
                }
            }
            catch (Exception e)
            {
                // Fallback to individual calls if batch fails
                if (progress)
                    progress("Batch failed, checking packages individually...");

                foreach (i, name; aurPackageNames)
                {
                    if (progress)
                        progress("Checking " ~ name ~ " (" ~ (i + 1)
                                .to!string ~ "/" ~ aurPackageNames.length.to!string ~ ")");

                    try
                    {
                        auto aurInfo = info(name);
                        if (aurInfo.name.length > 0 && aurInfo.ver.length > 0
                                && aurInfo.ver != packageVersions[name])
                        {
                            result ~= OutdatedPackage(name: name, fromVersion: packageVersions[name],
                        toVersion: aurInfo.ver, source: "aur");
                        }
                    }
                    catch (Exception)
                    {
                        // Skip packages that can't be queried
                        continue;
                    }
                }
            }
        }
    }

    return result;
}

SearchResult[] searchAny(string[] terms, PackageSource source = PackageSource.any)
{
    SearchResult[] results;

    if (source == PackageSource.repo || source == PackageSource.any)
    {
        try
        {
            results ~= searchRepo(terms);
        }
        catch (Exception)
        {
            // Tolerate repo search errors
        }
    }

    if (source == PackageSource.aur || source == PackageSource.any)
    {
        try
        {
            results ~= search(terms);
        }
        catch (Exception)
        {
            // Tolerate AUR/network errors
        }
    }

    return results;
}
