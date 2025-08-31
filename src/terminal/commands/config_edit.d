module terminal.commands.config_edit;

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
    string host = HostDetection.detect();
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