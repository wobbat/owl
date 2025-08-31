module config.analysis;

import std.algorithm;
import std.array;
import std.conv;
import std.format;
import std.stdio;
import std.string;
import terminal.colors;
import terminal.ui;
import terminal.args;
import utils.common;
import config.loader;
import config.parser;

struct ConfigStats
{
    size_t pkgCount;
    size_t dotCount;
    size_t envCount;
    size_t svcCount;
    size_t setupCount;
}

struct ConfigAnalysisResult
{
    string hostname;
    ConfigResult config;
    ConfigStats stats;
    string summaryLine;
}

ConfigAnalysisResult analyzeConfigChain(string hostname, string configDir)
{
    auto result = loadConfigChain(configDir, hostname);

    ConfigStats stats;
    foreach (entry; result.entries)
    {
        if (isSystemEntry(entry.pkgName))
            continue;

        stats.pkgCount++;
        stats.dotCount += entry.configs.length;
        stats.envCount += entry.envs.length;
        stats.svcCount += entry.services.length;
        stats.setupCount += entry.setups.length;
    }

    stats.envCount += result.globalEnvs.length;
    stats.setupCount += result.globalScripts.length;

    string summaryLine = format("Packages: %d | Dotfiles: %d | Envs: %d | Services: %d | Setups: %d",
            stats.pkgCount, stats.dotCount, stats.envCount, stats.svcCount, stats.setupCount);

    return ConfigAnalysisResult(hostname, result, stats, summaryLine);
}

bool isSystemEntry(string pkgName)
{
    return pkgName.startsWith(":") || pkgName.startsWith("@") || pkgName.startsWith("!");
}

void displayConfigAnalysis(ConfigAnalysisResult analysis)
{
    writeln(bold(cyan("Parsed Config Chain:")));
    writeln("Packages:");

    displayPackageEntries(analysis.config.entries);
    displayGlobalEnvironment(analysis.config.globalEnvs);
    displayGlobalScripts(analysis.config.globalScripts);
    displayGlobalServices(analysis.config.entries);
    displaySummary(analysis.summaryLine);
}

void displayPackageEntries(ConfigEntry[] entries)
{
    foreach (entry; entries)
    {
        if (isSystemEntry(entry.pkgName))
            continue;

        string line = bold(entry.pkgName) ~ " [" ~ entry.sourceFile ~ "]";

        if (entry.configs.length > 0)
        {
            line ~= " | configs: ";
            string[] cfgs;
            foreach (cfg; entry.configs)
                cfgs ~= cfg.source ~ "->" ~ cfg.dest;
            line ~= cfgs.join(", ");
        }

        if (entry.setups.length > 0)
        {
            line ~= " | setup: ";
            line ~= entry.setups.join(", ");
        }

        if (entry.services.length > 0)
        {
            line ~= " | service: ";
            line ~= entry.services.join(", ");
        }

        if (entry.envs.length > 0)
        {
            line ~= " | env: ";
            string[] envs;
            foreach (k, v; entry.envs)
                envs ~= k ~ "=" ~ v;
            line ~= envs.join(", ");
        }

        writeln(line);
    }
    writeln("");
}

void displayGlobalEnvironment(string[string] globalEnvs)
{
    writeln(bold(cyan("Global @env:")));
    string[] envs;
    foreach (k, v; globalEnvs)
        envs ~= k ~ "=" ~ v;
    if (envs.length > 0)
        writeln(envs.join(" | "));
    writeln("");
}

void displayGlobalScripts(string[] globalScripts)
{
    writeln(bold(cyan("Global Scripts:")));
    if (globalScripts.length > 0)
        writeln(globalScripts.join(" | "));
    writeln("");
}

void displayGlobalServices(ConfigEntry[] entries)
{
    auto globalServices = entries.filter!(e => e.pkgName == "__services__");
    foreach (entry; globalServices)
    {
        if (entry.services.length > 0)
        {
            writeln(bold(cyan("Global @service blocks:")));
            foreach (svc; entry.services)
            {
                writeln("  ", svc);
            }
        }
    }
    writeln("");
}

void displaySummary(string summaryLine)
{
    writeln(bold(cyan("Summary:")));
    writeln(summaryLine);
}

string resolveHostname(const CommandCall cc)
{
    if ("host" in cc.flags && cc.arguments.length > 0)
        return cc.arguments[0];
    return HostDetection.detect();
}
