module terminal.colors;

import std.string : format;

// Core color constants - semantic names instead of magic numbers
enum
{
    // Basic colors (30-37, 90-97)  
    ColorReset = "\x1b[0m",
    ColorBold = "\x1b[1m",
    ColorDim = "\x1b[2m",

    // Standard foreground colors
    ColorBlack = "\x1b[30m",
    ColorRed = "\x1b[31m",
    ColorGreen = "\x1b[32m",
    ColorYellow = "\x1b[33m",
    ColorBlue = "\x1b[34m",
    ColorMagenta = "\x1b[35m",
    ColorCyan = "\x1b[36m",
    ColorWhite = "\x1b[37m",

    // Bright foreground colors
    ColorBrightBlack = "\x1b[90m",
    ColorBrightRed = "\x1b[91m",
    ColorBrightGreen = "\x1b[92m",
    ColorBrightYellow = "\x1b[93m",
    ColorBrightBlue = "\x1b[94m",
    ColorBrightMagenta = "\x1b[95m",
    ColorBrightCyan = "\x1b[96m",
    ColorBrightWhite = "\x1b[97m"
}

// Semantic color aliases for common UI elements  
enum
{
    // Status colors
    Success = ColorGreen,
    ErrorColor = ColorRed,
    Warning = ColorYellow,
    Info = ColorBlue,

    // Package/content colors
    PackageName = ColorCyan,
    Version = ColorWhite,
    Repository = ColorBlue,
    Description = ColorWhite,

    // UI elements
    Bullet = ColorDim,
    Arrow = ColorDim,
    Highlight = ColorWhite,
    Muted = ColorDim,

    // Badges and sections
    BadgeText = ColorWhite,
    SectionBlue = ColorBlue,
    SectionRed = ColorRed,
    SectionYellow = ColorYellow,
    SectionMagenta = ColorMagenta,
    SectionTeal = ColorCyan
}

// Terminal control sequences
enum
{
    ClearLine = "\r\033[K", // Clear line and carriage return
    CarriageReturn = "\r",
    ClearToEndOfLine = "\033[K"
}

// RGB background colors for badges
enum
{
    BgRed = "\x1b[48;2;166;58;58m",
    BgYellow = "\x1b[48;2;255;179;101m",
    BgMagenta = "\x1b[48;2;140;104;106m",
    BgTeal = "\x1b[48;2;77;182;172m",
    BgBlue = "\x1b[48;2;104;119;140m",
    BgWhite = "\x1b[48;2;255;255;255m",

    FgBlack = "\x1b[38;2;0;0;0m",
    FgWhite = "\x1b[38;2;255;255;255m"
}

// Utility functions for applying colors
string colored(string text, string color)
{
    return color ~ text ~ ColorReset;
}

string bold(string text)
{
    return ColorBold ~ text ~ ColorReset;
}

string dim(string text)
{
    return ColorDim ~ text ~ ColorReset;
}

string badge(string text, string bg, string fg = FgWhite)
{
    return bg ~ fg ~ " " ~ text ~ " " ~ ColorReset;
}

// Semantic styling functions for common UI patterns
string successText(string text)
{
    return colored(text, Success);
}

string errorText(string text)
{
    return colored(text, ErrorColor);
}

string warningText(string text)
{
    return colored(text, Warning);
}

string infoText(string text)
{
    return colored(text, Info);
}

string packageName(string text)
{
    return colored(text, PackageName);
}

string versionText(string text)
{
    return colored(text, Version);
}

string repository(string text)
{
    return colored(text, Repository);
}

string description(string text)
{
    return colored(text, Description);
}

string highlight(string text)
{
    return colored(text, Highlight);
}

string muted(string text)
{
    return colored(text, Muted);
}

// Status symbols with consistent coloring
string symbolOk()
{
    return successText("+");
}

string symbolFail()
{
    return errorText("-");
}

string symbolInfo()
{
    return infoText("i");
}

string symbolWarn()
{
    return warningText("!");
}

string symbolArrow()
{
    return muted("->");
}

// Badge helpers for section headers
string redBadge(string text)
{
    return badge(text, BgRed, FgWhite);
}

string yellowBadge(string text)
{
    return badge(text, BgYellow, FgBlack);
}

string magentaBadge(string text)
{
    return badge(text, BgMagenta, FgWhite);
}

string tealBadge(string text)
{
    return badge(text, BgTeal, FgWhite);
}

string blueBadge(string text)
{
    return badge(text, BgBlue, FgWhite);
}

// Bracket helpers for common patterns
string brackets(string text, string color = Repository)
{
    return "[" ~ colored(text, color) ~ "]";
}

string numberBrackets(int num)
{
    return "[" ~ colored(format("%d", num), Success) ~ "]";
}

// Multi-color line helpers
string packageLine(string name, string ver, string repo)
{
    return packageName(name) ~ " " ~ versionText(ver) ~ " " ~ brackets(repo);
}

string upgradePackageLine(string name, string source = "")
{
    string src = source.length > 0 ? " (" ~ source ~ ")" : "";
    return "    " ~ warningText("upgrade") ~ " " ~ highlight(name) ~ src;
}

string installStatusLine(string pkg, string status, string timing)
{
    string statusColored;
    switch (status)
    {
    case "installed":
        statusColored = successText(status);
        break;
    case "failed":
        statusColored = errorText(status);
        break;
    default:
        statusColored = status;
        break;
    }
    return "  Package - " ~ statusColored ~ " " ~ dim(timing);
}

string dotfilesStatusLine(string status, string timing)
{
    string statusColored;
    switch (status)
    {
    case "up to date":
    case "synced":
        statusColored = successText(status);
        break;
    case "failed":
        statusColored = errorText(status);
        break;
    default:
        statusColored = status;
        break;
    }
    return "  Dotfiles - " ~ statusColored ~ " " ~ dim(timing);
}

import std.array : replicate;

// Separator and border helpers
string separator(string chr, int length, string color = SectionBlue)
{
    return colored(replicate(chr, length), color);
}

string borderLine(string text, string color = Success)
{
    string sep = separator(":", cast(int)(text.length + 4), color);
    return sep ~ "\n  " ~ highlight(text) ~ "\n" ~ sep;
}

// Color functions matching standard names
string red(string text)
{
    return colored(text, ColorRed);
}

string green(string text)
{
    return colored(text, ColorGreen);
}

string yellow(string text)
{
    return colored(text, ColorYellow);
}

string blue(string text)
{
    return colored(text, ColorBlue);
}

string magenta(string text)
{
    return colored(text, ColorMagenta);
}

string cyan(string text)
{
    return colored(text, ColorCyan);
}

string white(string text)
{
    return colored(text, ColorWhite);
}
