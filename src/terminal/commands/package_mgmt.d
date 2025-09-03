module terminal.commands.package_mgmt;

import terminal.commands.common_imports;

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

    auto results = searchAny(cast(string[]) searchTerms, options.source, options.paru);

    // Display results exactly like nim version
    writeln("\n" ~ bold("Found") ~ " " ~ format("%d", results.length) ~ " package(s):\n");

    if (results.length == 0)
    {
        writeln("");
        return 1;
    }

    // If paru was requested, preserve the order paru returned but invert numbering
    if (options.paru)
    {
        foreach (size_t i; 0 .. results.length)
        {
            auto result = results[i];

            string numStr = numberBrackets(cast(int)(results.length - i));
            string name = highlight(result.name);
            string versionStr = successText(result.ver);
            string tag = result.source == PackageSource.aur ? brackets("aur",
                    Warning) : brackets(result.repo, Repository);
            string status = result.installed ? " " ~ successText("installed") : "";
            string desc = result.description.length > 0 ? " - " ~ description(
                    result.description) : "";

            writeln(numStr ~ " " ~ name ~ " " ~ versionStr ~ " " ~ tag ~ status ~ desc);
        }
        writeln("");
    }
    else
    {
        foreach (size_t i; 0 .. results.length)
        {
            auto result = results[i];

            string numStr = numberBrackets(cast(int)(i + 1));
            string name = highlight(result.name);
            string versionStr = successText(result.ver);
            string tag = result.source == PackageSource.aur ? brackets("aur",
                    Warning) : brackets(result.repo, Repository);
            string status = result.installed ? " " ~ successText("installed") : "";
            string desc = result.description.length > 0 ? " - " ~ description(
                    result.description) : "";

            writeln(numStr ~ " " ~ name ~ " " ~ versionStr ~ " " ~ tag ~ status ~ desc);
        }
        writeln("");
    }

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

    // Map selection to result depending on display order
    auto chosen = options.paru ? results[results.length - selNum] : results[selNum - 1];
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

            foreach (size_t i; 0 .. files.length)
            {
                string file = files[i];
                string friendly = file.replace("~/", "");
                string numberPart = numberBrackets(cast(int)(i + 1));
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
            targetFile = files[pick - 1];
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

/// Run track command - track explicitly-installed packages into Owl configs
int runTrackCommand(const CommandCall cc)
{
    auto opts = parseCommandOptions(cc.flags, cc.arguments);
    return trackPackages(cc.arguments, opts);
}

/// Track explicitly-installed packages into configuration
int trackPackages(const string[] args, CommandOptions options)
{
    import utils.selection;

    string host = HostDetection.detect();
    auto candidates = computeTrackCandidates(host);

    if (candidates.length == 0)
    {
        ok("No untracked explicit packages found");
        return 0;
    }

    writeln("\n" ~ bold("Found") ~ " " ~ to!string(candidates.length) ~ " untracked package(s):\n");

    displayCountdownSelection(candidates, (string item, size_t num) {
        return packageSelectionFormatter(item, num);
    });

    auto packageResult = handleSelection(candidates);
    if (!packageResult.valid)
    {
        return 0;
    }

    string selected = packageResult.item;
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

        string homeDir = environment["HOME"];
        displayCountdownSelection(files, (string file, size_t num) {
            return fileSelectionFormatter(file, num, homeDir);
        });

        auto fileResult = handleSelection(files);
        if (!fileResult.valid)
        {
            errorOutput("Invalid selection");
            return 1;
        }
        targetFile = fileResult.item.replace(homeDir, "~");
    }

    addPackageToFile(selected, targetFile);
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
    string host = HostDetection.detect();
    auto candidates = computeTrackCandidates(host);

    if (candidates.length == 0)
    {
        ok("No candidates to hide");
        return 0;
    }

    writeln("\n" ~ bold("Candidate packages (hide to ignore in track):") ~ "\n");

    foreach (size_t i; 0 .. candidates.length)
    {
        string pkg = candidates[i];
        string numberPart = successText("[") ~ to!string(i + 1) ~ successText("]");
        writeln(numberPart ~ " " ~ packageName(pkg));
    }
    writeln("");

    int selection = promptSelection(cast(int) candidates.length);
    if (selection <= 0 || selection > candidates.length)
    {
        return 0;
    }

    string selected = candidates[selection - 1];
    addToUntracked(selected);
    import terminal.ui : success;

    success("Hidden '" ~ selected ~ "' from track suggestions");
    return 0;
}

/// Run dots command - check and sync only dotfiles configurations
int runDotsCommand(const CommandCall cc)
{
    auto opts = parseCommandOptions(cc.flags, cc.arguments);
    import terminal.commands.apply : dotsCheck;

    return dotsCheck(opts);
}
