module packages.aur_build;

import std.process;
import std.file;
import std.path;
import std.algorithm;
import std.stdio;
import std.string;
import core.time;
import core.thread;
import packages.types;
import utils.sh;
import std.stdio : writeln;

/// Run command with output suppression (no tick callback needed for threaded spinner)
int runCommandSilent(string[] args, string workDir = "")
{
    try
    {
        // Use pipeProcess to suppress output
        auto pipes = workDir.length > 0 ? pipeProcess(args, Redirect.stdout | Redirect.stderr,
                null, Config.none, workDir) : pipeProcess(args, Redirect.stdout | Redirect.stderr);

        scope (exit)
        {
            // Always clean up the process
            if (!pipes.pid.tryWait().terminated)
            {
                import core.sys.posix.signal;

                kill(pipes.pid.processID, SIGTERM);
            }
        }

        // Simple wait for completion - no polling needed since spinner is threaded
        auto result = wait(pipes.pid);
        return result;
    }
    catch (Exception e)
    {
        return 1;
    }
}

/// Clone AUR package repository
int cloneRepo(string name, string destDir, ProgressCallback progress = null)
{
    // Clean up any existing directory completely
    if (exists(destDir))
    {
        try
        {
            // Use sudo to clean up any files with permission issues from previous makepkg runs
            import utils.sh;

            run("sudo rm -rf " ~ destDir);
        }
        catch (Exception e)
        {
            // Fallback to regular removal
            try
            {
                rmdirRecurse(destDir);
            }
            catch (Exception)
            {
                // Ignore cleanup errors
            }
        }
    }

    try
    {
        mkdirRecurse(destDir);
    }
    catch (Exception e)
    {
        return 1;
    }

    string url = "https://aur.archlinux.org/" ~ name ~ ".git";
    string[] args = ["git", "clone", url, destDir];

    try
    {
        if (progress)
            progress("Downloading " ~ name ~ " from AUR...");

        return runCommandSilent(args);
    }
    catch (Exception e)
    {
        return 1;
    }
}

/// Build package using makepkg
int makepkg(string dir, bool clean = true, bool skipConfirm = true, ProgressCallback progress = null)
{
    string[] args = ["makepkg"];

    // Essential flags for AUR building
    args ~= "-s"; // Install missing dependencies 
    args ~= "-i"; // Install package after build
    args ~= "-C"; // Clean build directory

    if (skipConfirm)
        args ~= "--noconfirm";
    if (clean)
        args ~= "-f"; // Force overwrite existing package

    try
    {
        if (progress)
            progress("Downloading sources and building...");

        // Use silent command for clean spinner experience
        return runCommandSilent(args, dir);
    }
    catch (Exception e)
    {
        return 1;
    }
}

/// Find built package file in directory
string findBuiltPackageFile(string dir)
{
    try
    {
        foreach (DirEntry entry; dirEntries(dir, SpanMode.shallow))
        {
            if (entry.isFile && entry.name.endsWith(".pkg.tar.zst"))
            {
                return entry.name;
            }
        }
    }
    catch (Exception e)
    {
        // Directory doesn't exist or other error
    }
    return "";
}

/// Install built package using pacman
int installBuiltPackage(string pkgPath, bool assumeYes = true, ProgressCallback progress = null)
{
    string[] args = ["pacman", "-U", pkgPath];
    if (assumeYes)
        args ~= "--noconfirm";

    try
    {
        if (progress)
            progress("Installing package...");

        return runCommandSilent(args);
    }
    catch (Exception e)
    {
        return 1;
    }
}

/// Get AUR package status by trying a simple API call
bool checkAurStatus()
{
    try
    {
        import std.net.curl;

        auto content = get("https://aur.archlinux.org/rpc/v5/info?arg=bash");
        return content.length > 0;
    }
    catch (Exception e)
    {
        return false;
    }
}

/// Build and install AUR package with error handling and optional diff
bool buildAndInstallAurPackage(string name, ProgressCallback progress = null,
        bool showDiff = false, bool interactive = true)
{
    string tmpDir = tempDir() ~ "/aur-" ~ name;
    bool success = false;

    try
    {
        // Clone repository
        if (progress)
            progress("Cloning " ~ name ~ " repository...");
        if (cloneRepo(name, tmpDir, progress) != 0)
        {
            return false;
        }

        // Show PKGBUILD diff if requested
        if (showDiff)
        {
            import packages.pkgbuild;

            string diff = getPkgbuildDiff(name, tmpDir);
            writeln("\n" ~ diff ~ "\n");
        }

        // Interactive confirmation if requested
        if (interactive)
        {
            import terminal.prompt;

            bool proceed = confirmYesNo("Proceed with building " ~ name ~ "?", false);
            if (!proceed)
            {
                return false;
            }
        }

        // Build and install package (makepkg -si handles both)
        if (progress)
            progress("Building " ~ name ~ " (this may take a while)...");
        if (makepkg(tmpDir, true, true, progress) != 0)
        {
            return false;
        }

        if (progress)
            progress("Successfully installed " ~ name);
        success = true;
    }
    catch (Exception e)
    {
        success = false;
    }

    // Cleanup temporary directory
    if (exists(tmpDir))
    {
        try
        {
            // Use sudo to clean up any files with permission issues from makepkg runs
            import utils.sh;

            run("sudo rm -rf " ~ tmpDir);
        }
        catch (Exception e)
        {
            // Fallback to regular removal
            try
            {
                rmdirRecurse(tmpDir);
            }
            catch (Exception)
            {
                // Ignore cleanup errors
            }
        }
    }

    return success;
}
