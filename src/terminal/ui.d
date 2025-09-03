module terminal.ui;

import std.stdio;
import std.string;
import std.format;
import std.algorithm;
import std.array;
import core.time;
import core.thread;
import core.sync.mutex;
import core.atomic;
import terminal.colors;

// Spinner constants
enum SpinnerFrames = [
    "⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"
];
enum SpinnerIntervalMs = 120;

// Section header functions matching nim exactly
void sectionHeader(string section, string color = "blue")
{
    writeln("");
    string badgeText;
    switch (color)
    {
    case "red":
        badgeText = redBadge(section);
        break;
    case "yellow":
        badgeText = yellowBadge(section);
        break;
    case "magenta":
        badgeText = magentaBadge(section);
        break;
    case "teal":
        badgeText = tealBadge(section);
        break;
    case "orange":
        badgeText = orangeBadge(section);
        break;
    case "green":
        badgeText = greenBadge(section);
        break;
    default:
        badgeText = blueBadge(section);
        break;
    }
    writeln(badgeText);
    writeln("");
}

// Info section matching nim exactly
void overview(string host, int packages)
{
    write("  ");
    write(dim("host:"));
    write("     ");
    write(host);
    writeln("");
    write("  ");
    write(dim("packages:"));
    write(" ");
    write(format("%d", packages));
    writeln("");
}

// Install header
void installHeader()
{
    writeln(colored("Installing:", Info));
}

// Package install progress 
void packageInstallProgress(string pkgName)
{
    writeln(packageName(pkgName) ~ " " ~ symbolArrow());
}

// Success/status messages
void success(string text)
{
    writeln(borderLine(text, Success));
}

void normal(string text)
{
    writeln(highlight(" "));
    writeln(highlight(text));
    writeln(highlight(" "));
}

void error(string text)
{
    writeln("");
    stderr.write(colored(text, ErrorColor) ~ "\n");
    writeln("");
}

// Config management header exactly matching nim
void configManagementHeader()
{
    writeln("");
    writeln(magentaBadge("Config"));
    writeln("");
    writeln("  Config management:");
}

// Config packages summary
void configPackagesSummary(string summary)
{
    writeln("  " ~ packageName(summary) ~ " " ~ symbolArrow());
}

// Dotfiles status line
void showDotfilesUpToDate(int ms)
{
    writeln("  Dotfiles - " ~ successText("up to date") ~ " " ~ dim(format("(%dms)", ms)));
}

// Package removal display
void showPackagesRemoved(int count)
{
    writeln("  " ~ symbolOk() ~ " Removed " ~ format("%d", count) ~ " packages");
    writeln("");
}

// All packages upgraded
void showAllPackagesUpgraded()
{
    writeln("  " ~ symbolOk() ~ " All packages upgraded to latest versions");
}

// OK message with symbol
void ok(string msg)
{
    writeln("  " ~ symbolOk() ~ " " ~ msg);
}

// Spinner class with threaded animation
class Spinner
{
    private
    {
        bool enabled;
        shared bool running;
        string text;
        string colorCode;
        long startMs;
        int frameIdx;
        Thread spinnerThread;
        Mutex textMutex;
    }

    this(string text, bool enabled = true, string colorCode = ColorBlue)
    {
        this.enabled = enabled;
        this.text = text;
        this.colorCode = colorCode;
        this.startMs = nowMs();
        this.frameIdx = 0;
        this.textMutex = new Mutex();

        if (enabled)
        {
            atomicStore(running, true);
            spinnerThread = new Thread(&spinnerLoop);
            spinnerThread.start();
        }
    }

    private void spinnerLoop()
    {
        while (atomicLoad(running))
        {
            tick();
            Thread.sleep(dur!"msecs"(SpinnerIntervalMs));
        }
    }

    void tick()
    {
        if (!enabled || !atomicLoad(running))
            return;

        string currentText;
        synchronized (textMutex)
        {
            currentText = text;
        }

        string frame = SpinnerFrames[frameIdx % SpinnerFrames.length];
        frameIdx++;

        // Clear the line first to prevent leftover text  
        write(ClearLine);

        // Carriage return animation matching nim exactly
        if (currentText.canFind("Package - installing") || currentText.canFind("Dotfiles - "))
        {
            write(format("  %s %s  ", colored(frame, colorCode), currentText));
        }
        else
        {
            write(format("  %s  ", colored(frame ~ " " ~ currentText, colorCode)));
        }
        stdout.flush();
    }

    void update(string newText)
    {
        if (!enabled)
            return;

        synchronized (textMutex)
        {
            text = newText;
        }
    }

    void stop(string suffix = "")
    {
        if (enabled)
        {
            atomicStore(running, false);
            if (spinnerThread !is null)
            {
                spinnerThread.join();
            }
        }

        long duration = nowMs() - startMs;
        string timing = dim(format("(%dms)", duration));

        // Clear the line completely
        write(ClearLine);

        if (enabled)
        {
            string finalText;
            synchronized (textMutex)
            {
                finalText = text;
            }

            if (finalText.canFind("Package - installing"))
            {
                write(format("%s%s     \n", installStatusLine("Package",
                        "installed", timing), suffix.length > 0 ? " " ~ suffix : ""));
            }
            else if (finalText.canFind("Dotfiles - "))
            {
                string status = finalText.canFind("checking") ? "up to date" : "synced";
                write(format("%s%s     \n", dotfilesStatusLine(status, timing),
                        suffix.length > 0 ? " " ~ suffix : ""));
            }
            else
            {
                // Two-line format per legacy
                write(format("  %s %s\n", symbolOk(), highlight(finalText)));
                if (suffix.length > 0)
                {
                    writeln(format("    %s %s", dim(suffix), timing));
                }
                else
                {
                    writeln(format("    %s", timing));
                }
            }
        }
        else
        {
            string finalText;
            synchronized (textMutex)
            {
                finalText = text;
            }
            writeln(symbolOk() ~ " " ~ finalText);
        }
        stdout.flush();
    }

    void fail(string reason = "")
    {
        if (enabled)
        {
            atomicStore(running, false);
            if (spinnerThread !is null)
            {
                spinnerThread.join();
            }
        }

        long duration = nowMs() - startMs;
        string timing = dim(format("(%dms)", duration));

        // Clear the line completely
        write(ClearLine);

        string finalText;
        synchronized (textMutex)
        {
            finalText = text;
        }

        if (finalText.canFind("Package - installing"))
        {
            write(format("%s%s\n", installStatusLine("Package", "failed",
                    timing), reason.length > 0 ? " " ~ dim(reason) : ""));
        }
        else if (finalText.canFind("Dotfiles - "))
        {
            write(format("%s%s\n", dotfilesStatusLine("failed", timing),
                    reason.length > 0 ? " " ~ dim(reason) : ""));
        }
        else
        {
            write(format("  %s %s %s%s\n", symbolFail(), colored(finalText,
                    ErrorColor), timing, reason.length > 0 ? " " ~ dim(reason) : ""));
        }
        writeln("");
        stdout.flush();
    }
}

// Helper function to create new spinner
Spinner newSpinner(string text, bool enabled = true, string colorCode = ColorBlue)
{
    return new Spinner(text, enabled, colorCode);
}

// Get current time in milliseconds
long nowMs()
{
    import std.datetime.systime : Clock;
    import std.datetime.date : DateTime;

    auto now = Clock.currTime();
    return now.toUnixTime() * 1000 + now.fracSecs.total!"msecs";
}

// Helper functions for consistent messaging
string successMsg(string msg)
{
    return green("✓ " ~ msg);
}

string errorMsg(string msg)
{
    return red("✗ " ~ msg);
}

string warningMsg(string msg)
{
    return yellow("⚠ " ~ msg);
}

string infoMsg(string msg)
{
    return blue("ℹ " ~ msg);
}

// Standardized output functions to ensure consistency
void successOutput(string msg)
{
    writeln(successMsg(msg));
}

void errorOutput(string msg)
{
    stderr.writeln(errorMsg(msg));
}

void infoOutput(string msg)
{
    writeln(infoMsg(msg));
}

void warningOutput(string msg)
{
    writeln(warningMsg(msg));
}

// Consistent formatting for command results
void commandSuccess(string action, string details = "")
{
    string msg = action ~ (details.length > 0 ? ": " ~ details : "");
    successOutput(msg);
}

void commandError(string action, string details = "")
{
    string msg = action ~ (details.length > 0 ? ": " ~ details : "");
    errorOutput(msg);
}

void commandInfo(string action, string details = "")
{
    string msg = action ~ (details.length > 0 ? ": " ~ details : "");
    infoOutput(msg);
}
// Help UI structure
struct HelpUI
{
    bool useColor;

    this(bool useColor)
    {
        this.useColor = useColor;
    }

    void header(string txt)
    {
        writeln(useColor ? bold(cyan(txt)) : txt);
    }

    void subheader(string txt)
    {
        writeln(useColor ? bold(yellow(txt)) : txt);
    }

    void para(string txt)
    {
        writeln(txt);
    }

    void blank()
    {
        writeln();
    }

    void row(string left, string right, size_t leftWidth = 18)
    {
        string l = left.length >= leftWidth ? left ~ " " : leftJustify(left, leftWidth, ' ');
        writeln(useColor ? bold(l) ~ right : l ~ right);
    }

    void error(string txt)
    {
        writeln(useColor ? bold(red(txt)) : txt);
    }
}

void printTopLevelHelp()
{
    auto ui = HelpUI(true);
    ui.header("Usage");
    ui.para("  owl [--help] [--version]");
    ui.para("  owl apply [--no-aur] [--dev] [--no-paru] [--repo <name>]");
    ui.para("  owl [--no-aur] [--dev] [--no-paru] [--repo <name>]   # defaults to 'apply'");
    ui.para("  owl upgrade [--dev] [--no-paru]   # --dev includes VCS packages (-git, -hg, etc.)");
    ui.blank();

    ui.header("Description");
    ui.subheader("Top level flags");
    ui.row("  --help, -h", "Show this help and exit");
    ui.row("  --version, -v", "Show version and exit");
    ui.row("  --paru", "Use paru for package operations (default when available)");
    ui.row("  --no-paru", "Do not use paru for package operations");
    ui.blank();

    ui.subheader("Commands");
    ui.row("  apply", "Install packages, copy configs, and run setup scripts");
    ui.row("  dry-run, dr", "Preview what would be done without making changes");
    ui.row("  dots, d", "Check and sync only dotfiles configurations");
    ui.row("  track", "Track explicitly-installed packages into Owl configs");
    ui.row("  hide", "Hide packages from track suggestions");
    ui.row("  add", "Search for and add packages to configuration files");
    ui.row("  configedit, ce", "Edit configuration files with your preferred editor");
    ui.row("  dotedit, de", "Edit dotfiles with your preferred editor");
    ui.row("  upgrade, up", "Upgrade all packages to latest versions");
    ui.row("  check", "Print parsed config chain for debugging");
    ui.row("  help, --help, -h", "Show this help message");
    ui.row("  version, --version, -v", "Show version information");
    ui.blank();

    ui.header("Examples");
    ui.para("  owl --version");
    ui.para("  owl apply --no-aur");
    ui.para("  owl apply --no-paru   # Do not use paru for package operations");
    ui.para("  owl --no-aur");
    ui.para("  owl upgrade --dev   # Include VCS packages");
}

void printUnknownCommand(string cmd)
{
    auto ui = HelpUI(true);
    ui.error("Unknown command: " ~ cmd);
    ui.blank();
}

// Helper functions for spinner status lines
string installStatusLine(string prefix, string status, string timing)
{
    return format("  %s - %s %s", prefix, status, timing);
}

string dotfilesStatusLine(string status, string timing)
{
    return format("  Dotfiles - %s %s", successText(status), timing);
}
