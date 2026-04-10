#![allow(dead_code)]

/// ANSI color codes for terminal output
#[derive(Debug, Clone, Copy)]
pub enum Color {
    Red,
    Green,
    Yellow,
    Orange,
    EnvOrange,
    SystemPurple,
    Blue,
    Magenta,
    Cyan,
    Teal,
    White,
    BgRed,
    BgGreen,
    BgYellow,
    BgBlue,
    BgMagenta,
    BgCyan,
    BgWhite,
    Bold,
    Italic,
    Underline,
    Dim,
    Highlight,
    Success,
    Warning,
    Repository,
    Description,
}

/// ANSI color code mappings
impl Color {
    const fn ansi_code(self) -> &'static str {
        match self {
            Color::Red => "31",
            Color::Green => "32",
            Color::Yellow => "33",
            Color::Orange => "38;5;208",
            Color::EnvOrange => "38;5;166",
            Color::SystemPurple => "38;5;97",
            Color::Blue => "34",
            Color::Magenta => "35",
            Color::Cyan => "36",
            Color::Teal => "38;5;37",
            Color::White => "37",
            Color::BgRed => "41",
            Color::BgGreen => "42",
            Color::BgYellow => "43",
            Color::BgBlue => "44",
            Color::BgMagenta => "45",
            Color::BgCyan => "46",
            Color::BgWhite => "47",
            Color::Bold => "1",
            Color::Italic => "3",
            Color::Underline => "4",
            Color::Dim => "2",
            Color::Highlight => "1;36",
            Color::Success => "1;32",
            Color::Warning => "1;33",
            Color::Repository => "1;35",
            Color::Description => "2;37",
        }
    }
}

/// Apply ANSI color codes to text
pub fn colorize(s: &str, color: Color) -> String {
    format!("\x1b[{}m{}\x1b[0m", color.ansi_code(), s)
}

// Convenience functions for backward compatibility
pub fn red(s: &str) -> String {
    colorize(s, Color::Red)
}
pub fn green(s: &str) -> String {
    colorize(s, Color::Green)
}
pub fn yellow(s: &str) -> String {
    colorize(s, Color::Yellow)
}
pub fn orange(s: &str) -> String {
    colorize(s, Color::Orange)
}
pub fn env_orange(s: &str) -> String {
    colorize(s, Color::EnvOrange)
}
pub fn system_purple(s: &str) -> String {
    colorize(s, Color::SystemPurple)
}
pub fn blue(s: &str) -> String {
    colorize(s, Color::Blue)
}
pub fn magenta(s: &str) -> String {
    colorize(s, Color::Magenta)
}
pub fn cyan(s: &str) -> String {
    colorize(s, Color::Cyan)
}
pub fn teal(s: &str) -> String {
    colorize(s, Color::Teal)
}
pub fn white(s: &str) -> String {
    colorize(s, Color::White)
}
pub fn bg_red(s: &str) -> String {
    colorize(s, Color::BgRed)
}
pub fn bg_green(s: &str) -> String {
    colorize(s, Color::BgGreen)
}
pub fn bg_yellow(s: &str) -> String {
    colorize(s, Color::BgYellow)
}
pub fn bg_blue(s: &str) -> String {
    colorize(s, Color::BgBlue)
}
pub fn bg_magenta(s: &str) -> String {
    colorize(s, Color::BgMagenta)
}
pub fn bg_cyan(s: &str) -> String {
    colorize(s, Color::BgCyan)
}
pub fn bg_white(s: &str) -> String {
    colorize(s, Color::BgWhite)
}
pub fn bold(s: &str) -> String {
    colorize(s, Color::Bold)
}
pub fn italic(s: &str) -> String {
    colorize(s, Color::Italic)
}
pub fn underline(s: &str) -> String {
    colorize(s, Color::Underline)
}
pub fn dim(s: &str) -> String {
    colorize(s, Color::Dim)
}
pub fn highlight(s: &str) -> String {
    colorize(s, Color::Highlight)
}
pub fn success(s: &str) -> String {
    colorize(s, Color::Success)
}
pub fn warning(s: &str) -> String {
    colorize(s, Color::Warning)
}
pub fn repository(s: &str) -> String {
    colorize(s, Color::Repository)
}
pub fn description(s: &str) -> String {
    colorize(s, Color::Description)
}
