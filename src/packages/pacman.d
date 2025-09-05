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
private struct ProcessTracker {
    int[] pids;

    void register(int pid) {
        pids ~= pid;
    }

    void unregister(int pid) {
        import std.algorithm.mutation : remove;
        pids = pids.remove!(p => p == pid);
    }
}

private ProcessTracker pacmanTracker;

// Signal handler setup to clean up processes
static this() {
    import core.sys.posix.signal;
    import core.stdc.stdlib;

    extern (C) void signalHandler(int sig) nothrow @nogc @system {
        // Kill any active pacman processes
        foreach (pid; pacmanTracker.pids) {
            try {
                kill(pid, SIGTERM);
            } catch (Exception) {
                // Ignore errors during cleanup
            }
        }
        exit(1);
    }

    signal(SIGINT, &signalHandler);
    signal(SIGTERM, &signalHandler);
}

struct PacmanManager {
    bool unsafe;
    bool bypassCache;
    bool noAur;
    bool useParu;
}

PacmanManager initPacmanManager(bool unsafe = false, bool bypassCache = false,
        bool noAur = false, bool useParu = false) {
    return PacmanManager(unsafe, bypassCache, noAur, useParu);
}

bool ensureAvailable() {
    try {
        string output = run("which pacman");
        return output.length > 0;
    } catch (Exception) {
        return false;
    }
}

bool isParuInstalled() {
    try {
        auto result = execute(["which", "paru"]);
        return result.status == 0;
    } catch (Exception) {
        return false;
    }
}

void installParu() {
    import std.stdio : writeln, write, stdout;
    import std.process : spawnProcess, wait, Config, Redirect;

    writeln("Paru not found. Installing paru from AUR...");

    try {
        // Clean up any existing directory first
        writeln("Cleaning up any existing paru build directory...");
        auto cleanupPipes = pipeProcess(["rm", "-rf", "/tmp/paru"],
                Redirect.stdout | Redirect.stderr);
        wait(cleanupPipes.pid);

        // Clone the repo
        writeln("Cloning paru repository...");
        auto clonePipes = pipeProcess([
            "git", "clone", "https://aur.archlinux.org/paru.git", "/tmp/paru"
        ], Redirect.stdout | Redirect.stderr);

        // Stream output to user
        foreach (line; clonePipes.stdout.byLine) {
            writeln(line);
        }

        auto cloneStatus = wait(clonePipes.pid);
        if (cloneStatus != 0) {
            throw new Exception("Failed to clone paru repo");
        }

        // Build and install in the directory
        writeln("Building and installing paru...");
        auto buildPipes = pipeProcess(["makepkg", "-si", "--noconfirm"],
                Redirect.stdout | Redirect.stderr, null, Config.none, "/tmp/paru");

        // Stream output to user
        foreach (line; buildPipes.stdout.byLine) {
            writeln(line);
        }

        auto buildStatus = wait(buildPipes.pid);
        if (buildStatus != 0) {
            throw new Exception("Failed to build paru");
        }

        // Clean up
        writeln("Cleaning up...");
        auto rmPipes = pipeProcess(["rm", "-rf", "/tmp/paru"], Redirect.stdout | Redirect.stderr);
        wait(rmPipes.pid); // Don't check status for cleanup

        writeln("Paru installation completed successfully!");
    } catch (Exception e) {
        writeln("Failed to install paru: ", e.msg);
        throw e;
    }
}

bool ensureParuAvailable() {
    if (isParuInstalled()) {
        return true;
    }
    installParu();
    if (!isParuInstalled()) {
        throw new Exception("Paru installation failed: paru not found after installation");
    }
    return true;
}

int runPacman(string[] args, ProgressCallback onLine = null, bool useParu = false) {
    return runPacmanStream(args, onLine, null, useParu);
}

private string[] buildPacmanCommand(string[] args, bool useParu) {
    bool paruAvailable = false;
    if (useParu) {
        paruAvailable = ensureParuAvailable();
    }

    string packageManager = paruAvailable ? "paru" : "pacman";

    // Prefer running via sudo if available (only for pacman, paru handles sudo internally)
    try {
        string output = run("which sudo");
        if (output.length > 0 && !paruAvailable) {
            return ["sudo", packageManager] ~ args;
        } else {
            return [packageManager] ~ args;
        }
    } catch (Exception) {
        return [packageManager] ~ args;
    }
}

private int streamProcessOutput(T)(T pipe, ProgressCallback onLine, void delegate() onTick) {
    import core.time : dur, MonoTime;
    import core.thread : Thread;

    auto lastTickTime = MonoTime.currTime();
    enum tickIntervalMs = 80;
    enum idleSleepMs = 10;

    char[] lineBuffer;
    while (pipe.pid.tryWait().terminated == false || !pipe.stdout.eof()) {
        bool gotLine = false;

        // Try to read a line without blocking
        try {
            if (pipe.stdout.readln(lineBuffer)) {
                gotLine = true;
                if (onLine) {
                    onLine(lineBuffer.idup.chomp());
                }
            }
        } catch (Exception) {
            // End of stream or error
            break;
        }

        // Check if we should tick
        auto now = MonoTime.currTime();
        if (onTick && (now - lastTickTime) >= dur!"msecs"(tickIntervalMs)) {
            onTick();
            lastTickTime = now;
        }

        // If we didn't get a line, sleep briefly to avoid busy waiting
        if (!gotLine) {
            Thread.sleep(dur!"msecs"(idleSleepMs));
        }
    }

    // Wait for process to complete
    return wait(pipe.pid);
}

int runPacmanStream(string[] args, ProgressCallback onLine = null,
        void delegate() onTick = null, bool useParu = false) {
    string[] cmd = buildPacmanCommand(args, useParu);

    try {
        // Use pipeProcess for better control and cleanup
        auto pipes = pipeProcess(cmd, Redirect.stdout | Redirect.stderr);

        // Track this process for cleanup
        int pidNum = pipes.pid.processID;
        pacmanTracker.register(pidNum);

        scope (exit) {
            // Remove from tracking when done
            pacmanTracker.unregister(pidNum);
        }

        scope (failure) {
            // Kill the process if something goes wrong
            import core.sys.posix.signal;

            try {
                kill(pidNum, SIGTERM);
                wait(pipes.pid); // Clean up zombie
            } catch (Exception) {
            }
        }

        return streamProcessOutput(pipes, onLine, onTick);
    } catch (Exception e) {
        stderr.writeln("Error running pacman/paru: ", e.msg);
        return 1;
    }
}

string[2][] getInstalledPackages() {
    try {
        string output = run("pacman -Q");
        return parsePacmanOutput(output);
    } catch (Exception) {
        return [];
    }
}

string[2][] getExplicitlyInstalledPackages() {
    try {
        string output = run("pacman -Qe");
        return parsePacmanOutput(output);
    } catch (Exception) {
        return [];
    }
}

string[2][] getForeignPackages() {
    try {
        string output = run("pacman -Qm");
        return parsePacmanOutput(output);
    } catch (Exception) {
        return [];
    }
}

OutdatedPackage[] getOutdatedRepoPackages() {
    try {
        string output = run("pacman -Qu");
        OutdatedPackage[] outdated;
        foreach (line; output.splitLines()) {
            if (line.length == 0)
                continue;
            auto parts = line.split();
            // format: name version -> newversion
            if (parts.length >= 4 && parts[2] == "->") {
                outdated ~= OutdatedPackage(name: parts[0], fromVersion: parts[1],
            toVersion: parts[3], source: "repo");
            }
        }
        return outdated;
    } catch (Exception) {
        return [];
    }
}

SearchResult[] searchRepo(string[] terms, bool useParu = false) {
    if (terms.length == 0)
        return [];

    try {
        // Check if paru should be used and is available
        bool paruAvailable = false;
        if (useParu) {
            paruAvailable = ensureParuAvailable();
        }

        string packageManager = paruAvailable ? "paru" : "pacman";
        string cmd = packageManager ~ " -Ss " ~ terms.join(" ");
        // Note: --bottomup only shows AUR packages, so we don't use it here
        // to get both official repos and AUR packages
        string output = run(cmd);

        SearchResult[] results;
        string curName = "";
        string curVer = "";
        string curRepo = "";
        string curDesc = "";

        foreach (line; output.splitLines()) {
            if (line.length == 0)
                continue;

            if (line.canFind("/") && line.canFind(" ") && !line.startsWith(" ")) {
                // flush previous
                if (curName.length > 0) {
                    results ~= SearchResult(name: curName, ver: curVer, source: PackageSource.repo,
                repo: curRepo, description: curDesc, installed: false);
                }

                // line like: repo/name version [installed: x]
                auto repoName = line.split(" ")[0];
                auto rv = repoName.split("/");
                if (rv.length == 2) {
                    curRepo = rv[0];
                    curName = rv[1];
                }
                auto rest = line[repoName.length .. $].strip();
                if (rest.length > 0) {
                    curVer = rest.split()[0];
                }
                curDesc = "";
            } else if (line.startsWith("    ")) {
                curDesc = line.strip();
            }
        }

        if (curName.length > 0) {
            results ~= SearchResult(name: curName, ver: curVer, source: PackageSource.repo,
        repo: curRepo, description: curDesc, installed: false);
        }

        return results;
    } catch (Exception) {
        return [];
    }
}

string getPackageRepository(string name) {
    try {
        string output = run("pacman -Qi " ~ name);
        foreach (line; output.splitLines()) {
            if (line.startsWith("Repository")) {
                auto parts = line.split(":");
                if (parts.length >= 2) {
                    return parts[1].strip();
                }
            }
        }
        return "repo";
    } catch (Exception) {
        return "repo";
    }
}

int installRepoPackages(string[] names, bool assumeYes = false, bool needed = false,
        ProgressCallback progress = null, void delegate() onTick = null, bool useParu = false) {
    if (names.length == 0)
        return 0;

    // Avoid circular dependency: don't use paru to install paru
    import std.algorithm : canFind;

    if (names.canFind("paru")) {
        useParu = false;
    }

    string[] args = ["-S"];
    if (assumeYes)
        args ~= "--noconfirm";
    if (needed)
        args ~= "--needed";
    args ~= names;

    return runPacmanStream(args, progress, onTick, useParu);
}

int removePackages(string[] names, bool recursive = false, bool assumeYes = false,
        ProgressCallback progress = null, bool useParu = false) {
    if (names.length == 0)
        return 0;

    string[] args = ["-R"];
    if (recursive)
        args ~= "-s";
    if (assumeYes)
        args ~= "--noconfirm";
    args ~= names;

    return runPacman(args, progress, useParu);
}

int syncDatabases(ProgressCallback progress = null, void delegate() onTick = null,
        bool useParu = false) {
    string[] args = ["-Sy"];
    return runPacmanStream(args, progress, onTick, useParu);
}

int syncDatabases(bool force = false, ProgressCallback progress = null,
        void delegate() onTick = null, bool useParu = false) {
    string[] args = ["-Sy"];
    if (force)
        args ~= "y"; // pacman -Syy

    return runPacmanStream(args, progress, onTick, useParu);
}

bool upgradeSystem(ProgressCallback progress = null, void delegate() onTick = null,
        bool useParu = false) {
    string[] args = ["-Su", "--noconfirm"];
    return runPacmanStream(args, progress, onTick, useParu) == 0;
}

int upgradeSystem(ref PacmanManager pm, bool verbose,
        ProgressCallback progress = null, void delegate() onTick = null) {
    string[] args = ["-Su"];
    if (!verbose)
        args ~= "--noconfirm";

    return runPacmanStream(args, progress, onTick, pm.useParu);
}
