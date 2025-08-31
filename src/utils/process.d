module utils.process;

import std.process;
import std.string;
import std.format;

/// Result of process execution
struct ProcessResult
{
    int exitCode;
    string errorMessage;
    bool success() const
    {
        return exitCode == 0;
    }
}

/// Run editor with given file path
ProcessResult runEditor(string editorBin, string filePath)
{
    if (editorBin.length == 0)
    {
        return ProcessResult(1, "EDITOR environment variable is not set");
    }

    try
    {
        auto pid = spawnProcess([editorBin, filePath]);
        int exitCode = wait(pid);
        return ProcessResult(exitCode, "");
    }
    catch (Exception e)
    {
        return ProcessResult(1, format("Failed to open editor: %s", e.msg));
    }
}

/// Check if editor is available
bool isEditorAvailable()
{
    string editorBin = environment.get("EDITOR", "");
    return editorBin.length > 0;
}

/// Get editor binary name from environment
string getEditorBinary()
{
    return environment.get("EDITOR", "");
}

/// Run any external command with arguments
ProcessResult runCommand(string[] command)
{
    if (command.length == 0)
    {
        return ProcessResult(1, "No command provided");
    }

    try
    {
        auto pid = spawnProcess(command);
        int exitCode = wait(pid);
        return ProcessResult(exitCode, "");
    }
    catch (Exception e)
    {
        return ProcessResult(1, format("Failed to run command '%s': %s", command[0], e.msg));
    }
}
