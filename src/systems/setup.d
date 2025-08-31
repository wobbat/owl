module systems.setup;

import std.algorithm;
import std.array;
import std.conv;
import std.file;
import std.path;
import std.process;
import std.stdio;
import std.string;
import terminal.ui;
import terminal.colors;

struct SetupResult
{
    bool success;
    string output;
    int exitCode;
}

/// Execute a setup script
SetupResult runSetupScript(string scriptPath)
{
    SetupResult result;

    try
    {
        // Check if script exists
        if (!exists(scriptPath))
        {
            result.success = false;
            result.output = "Script not found: " ~ scriptPath;
            result.exitCode = 1;
            return result;
        }

        // Make script executable if it's not
        if (isFile(scriptPath))
        {
            version (Posix)
            {
                import std.conv : octal;

                setAttributes(scriptPath, octal!755);
            }
        }

        // Execute the script
        auto pipes = pipeProcess([scriptPath], Redirect.stdout | Redirect.stderr);
        scope (exit)
            wait(pipes.pid);

        string output = "";
        foreach (line; pipes.stdout.byLine)
        {
            output ~= line.idup ~ "\n";
        }

        foreach (line; pipes.stderr.byLine)
        {
            output ~= line.idup ~ "\n";
        }

        result.exitCode = wait(pipes.pid);
        result.success = (result.exitCode == 0);
        result.output = output;
    }
    catch (Exception e)
    {
        result.success = false;
        result.output = "Failed to execute script: " ~ e.msg;
        result.exitCode = 1;
    }

    return result;
}

/// Run multiple setup scripts
bool runSetupScripts(string[] scripts)
{
    if (scripts.length == 0)
        return true;

    sectionHeader("Setup", "green");

    bool allSuccess = true;
    foreach (script; scripts)
    {
        string resolvedPath = script;

        // If not absolute path, resolve relative to .owl directory
        if (!isAbsolute(script))
        {
            import std.process : environment;

            string owlRoot = buildPath(environment["HOME"], ".owl");
            resolvedPath = buildPath(owlRoot, script);
        }

        writeln("  Running: " ~ script);
        auto result = runSetupScript(resolvedPath);

        if (result.success)
        {
            writeln("  " ~ successText("✓") ~ " " ~ script ~ " completed successfully");
        }
        else
        {
            writeln("  " ~ errorText(
                    "✗") ~ " " ~ script ~ " failed (exit code " ~ result.exitCode.to!string ~ ")");
            if (result.output.length > 0)
            {
                writeln("    Output: " ~ result.output.strip);
            }
            allSuccess = false;
        }
    }

    writeln("");
    if (allSuccess)
    {
        writeln("  " ~ successText("✓") ~ " All setup scripts completed successfully");
    }
    else
    {
        writeln("  " ~ warningText("!") ~ " Some setup scripts failed");
    }
    writeln("");

    return allSuccess;
}
