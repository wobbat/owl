module packages.pacman;

import std.algorithm;
import std.array;
import std.process;
import std.range;
import std.string;
import std.stdio;
import utils.sh;
import packages.types;

// Global process tracking for cleanup
private int[] activePacmanProcessIDs;

// Signal handler setup to clean up processes
static this()
{
    import core.sys.posix.signal;
    import core.stdc.stdlib;

    extern (C) void signalHandler(int sig) nothrow @nogc @system
    {
        // Kill any active pacman processes
        foreach (pid; activePacmanProcessIDs)
        {
            try
            {
                kill(pid, SIGTERM);
            }
            catch (Exception)
            {
                // Ignore errors during cleanup
            }
        }
        exit(1);
    }

    signal(SIGINT, &signalHandler);
    signal(SIGTERM, &signalHandler);
}

struct PacmanManager
{
    bool unsafe;
    bool bypassCache;
    bool noAur;
}

PacmanManager initPacmanManager(bool unsafe = false, bool bypassCache = false, bool noAur = false)
{
    return PacmanManager(unsafe, bypassCache, noAur);
}

bool ensureAvailable()
{
    try
    {
        string output = run("which pacman");
        return output.length > 0;
    }
    catch (Exception)
    {
        return false;
    }
}

int runPacman(string[] args, ProgressCallback onLine = null)
{
    return runPacmanStream(args, onLine, null);
}

int runPacmanStream(string[] args, ProgressCallback onLine = null, void delegate() onTick = null)
{
    string[] cmd;

    // Prefer running via sudo if available
    try
    {
        string output = run("which sudo");
        if (output.length > 0)
        {
            cmd = ["sudo", "pacman"] ~ args;
        }
        else
        {
            cmd = ["pacman"] ~ args;
        }
    }
    catch (Exception)
    {
        cmd = ["pacman"] ~ args;
    }

    try
    {
        import core.time : dur, MonoTime;
        import core.thread : Thread;

        // Use pipeProcess for better control and cleanup
        auto pipes = pipeProcess(cmd, Redirect.stdout | Redirect.stderr);

        // Track this process for cleanup
        int pidNum = pipes.pid.processID;
        activePacmanProcessIDs ~= pidNum;

        scope (exit)
        {
            // Remove from tracking when done
            import std.algorithm.mutation : remove;

            activePacmanProcessIDs = activePacmanProcessIDs.remove!(p => p == pidNum);
        }

        scope (failure)
        {
            // Kill the process if something goes wrong
            import core.sys.posix.signal;

            try
            {
                kill(pidNum, SIGTERM);
                wait(pipes.pid); // Clean up zombie
            }
            catch (Exception)
            {
            }
        }

        // Stream processing with tick support
        auto lastTickTime = MonoTime.currTime();
        enum tickIntervalMs = 80;
        enum idleSleepMs = 10;

        char[] lineBuffer;
        while (pipes.pid.tryWait().terminated == false || !pipes.stdout.eof())
        {
            bool gotLine = false;

            // Try to read a line without blocking
            try
            {
                if (pipes.stdout.readln(lineBuffer))
                {
                    gotLine = true;
                    if (onLine)
                    {
                        onLine(lineBuffer.idup.chomp());
                    }
                }
            }
            catch (Exception)
            {
                // End of stream or error
                break;
            }

            // Check if we should tick
            auto now = MonoTime.currTime();
            if (onTick && (now - lastTickTime) >= dur!"msecs"(tickIntervalMs))
            {
                onTick();
                lastTickTime = now;
            }

            // If we didn't get a line, sleep briefly to avoid busy waiting
            if (!gotLine)
            {
                Thread.sleep(dur!"msecs"(idleSleepMs));
            }
        }

        // Wait for process to complete
        auto result = wait(pipes.pid);
        return result;
    }
    catch (Exception e)
    {
        stderr.writeln("Error running pacman: ", e.msg);
        return 1;
    }
}

string[2][] getInstalledPackages()
{
    try
    {
        string output = run("pacman -Q");
        string[2][] packages;
        foreach (line; output.splitLines())
        {
            if (line.length == 0)
                continue;
            auto parts = line.split();
            if (parts.length >= 2)
            {
                packages ~= [parts[0], parts[1]];
            }
        }
        return packages;
    }
    catch (Exception)
    {
        return [];
    }
}

string[2][] getExplicitlyInstalledPackages()
{
    try
    {
        string output = run("pacman -Qe");
        string[2][] packages;
        foreach (line; output.splitLines())
        {
            if (line.length == 0)
                continue;
            auto parts = line.split();
            if (parts.length >= 2)
            {
                packages ~= [parts[0], parts[1]];
            }
        }
        return packages;
    }
    catch (Exception)
    {
        return [];
    }
}

string[2][] getForeignPackages()
{
    try
    {
        string output = run("pacman -Qm");
        string[2][] packages;
        foreach (line; output.splitLines())
        {
            if (line.length == 0)
                continue;
            auto parts = line.split();
            if (parts.length >= 2)
            {
                packages ~= [parts[0], parts[1]];
            }
        }
        return packages;
    }
    catch (Exception)
    {
        return [];
    }
}

OutdatedPackage[] getOutdatedRepoPackages()
{
    try
    {
        string output = run("pacman -Qu");
        OutdatedPackage[] outdated;
        foreach (line; output.splitLines())
        {
            if (line.length == 0)
                continue;
            auto parts = line.split();
            // format: name version -> newversion
            if (parts.length >= 4 && parts[2] == "->")
            {
                outdated ~= OutdatedPackage(name: parts[0], fromVersion: parts[1],
            toVersion: parts[3], source: "repo");
            }
        }
        return outdated;
    }
    catch (Exception)
    {
        return [];
    }
}

SearchResult[] searchRepo(string[] terms)
{
    if (terms.length == 0)
        return [];

    try
    {
        string cmd = "pacman -Ss " ~ terms.join(" ");
        string output = run(cmd);

        SearchResult[] results;
        string curName = "";
        string curVer = "";
        string curRepo = "";
        string curDesc = "";

        foreach (line; output.splitLines())
        {
            if (line.length == 0)
                continue;

            if (line.canFind("/") && line.canFind(" ") && !line.startsWith(" "))
            {
                // flush previous
                if (curName.length > 0)
                {
                    results ~= SearchResult(name: curName, ver: curVer, source: PackageSource.repo,
                repo: curRepo, description: curDesc, installed: false);
                }

                // line like: repo/name version [installed: x]
                auto repoName = line.split(" ")[0];
                auto rv = repoName.split("/");
                if (rv.length == 2)
                {
                    curRepo = rv[0];
                    curName = rv[1];
                }
                auto rest = line[repoName.length .. $].strip();
                if (rest.length > 0)
                {
                    curVer = rest.split()[0];
                }
                curDesc = "";
            }
            else if (line.startsWith("    "))
            {
                curDesc = line.strip();
            }
        }

        if (curName.length > 0)
        {
            results ~= SearchResult(name: curName, ver: curVer, source: PackageSource.repo,
        repo: curRepo, description: curDesc, installed: false);
        }

        return results;
    }
    catch (Exception)
    {
        return [];
    }
}

string getPackageRepository(string name)
{
    try
    {
        string output = run("pacman -Qi " ~ name);
        foreach (line; output.splitLines())
        {
            if (line.startsWith("Repository"))
            {
                auto parts = line.split(":");
                if (parts.length >= 2)
                {
                    return parts[1].strip();
                }
            }
        }
        return "repo";
    }
    catch (Exception)
    {
        return "repo";
    }
}

int installRepoPackages(string[] names, bool assumeYes = false, bool needed = false,
        ProgressCallback progress = null, void delegate() onTick = null)
{
    if (names.length == 0)
        return 0;

    string[] args = ["-S"];
    if (assumeYes)
        args ~= "--noconfirm";
    if (needed)
        args ~= "--needed";
    args ~= names;

    return runPacmanStream(args, progress, onTick);
}

int removePackages(string[] names, bool recursive = false, bool assumeYes = false,
        ProgressCallback progress = null)
{
    if (names.length == 0)
        return 0;

    string[] args = ["-R"];
    if (recursive)
        args ~= "-s";
    if (assumeYes)
        args ~= "--noconfirm";
    args ~= names;

    return runPacman(args, progress);
}

int syncDatabases(ProgressCallback progress = null, void delegate() onTick = null)
{
    string[] args = ["-Sy"];
    return runPacmanStream(args, progress, onTick);
}

int syncDatabases(bool force = false, ProgressCallback progress = null, void delegate() onTick = null)
{
    string[] args = ["-Sy"];
    if (force)
        args ~= "y"; // pacman -Syy

    return runPacmanStream(args, progress, onTick);
}

bool upgradeSystem(ProgressCallback progress = null, void delegate() onTick = null)
{
    string[] args = ["-Su", "--noconfirm"];
    return runPacmanStream(args, progress, onTick) == 0;
}

int upgradeSystem(ref PacmanManager pm, bool verbose,
        ProgressCallback progress = null, void delegate() onTick = null)
{
    string[] args = ["-Su"];
    if (!verbose)
        args ~= "--noconfirm";

    return runPacmanStream(args, progress, onTick);
}
