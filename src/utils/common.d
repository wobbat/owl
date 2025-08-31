module utils.common;

import std.algorithm;
import std.array;
import std.conv;
import std.format;
import std.stdio;
import std.string;
import terminal.ui;
import terminal.args;
import terminal.options;
import packages.types : ProgressCallback;

struct SpinnerContext
{
    bool noSpinner;
    bool verbose;

    this(bool noSpinner, bool verbose)
    {
        this.noSpinner = noSpinner;
        this.verbose = verbose;
    }

    this(CommandOptions options)
    {
        this.noSpinner = options.noSpinner;
        this.verbose = options.verbose;
    }
}

T withSpinner(T)(string initialText, SpinnerContext ctx, T delegate() operation)
{
    auto spinner = newSpinner(initialText, !ctx.noSpinner && !ctx.verbose);

    scope (exit)
    {
        if (!ctx.noSpinner && !ctx.verbose)
        {
            spinner.stop("");
        }
    }

    return operation();
}

void withSpinnerAction(string initialText, SpinnerContext ctx,
        void delegate(ProgressCallback) operation, string successMsg = "", string failMsg = "")
{
    auto spinner = newSpinner(initialText, !ctx.noSpinner && !ctx.verbose);

    ProgressCallback progress = (string msg) {
        if (!ctx.noSpinner && !ctx.verbose)
        {
            spinner.update(msg);
        }
    };

    try
    {
        operation(progress);
        if (!ctx.verbose)
        {
            spinner.stop(successMsg);
        }
    }
    catch (Exception e)
    {
        if (!ctx.verbose)
        {
            spinner.fail(failMsg.length > 0 ? failMsg : "failed");
        }
        throw e;
    }
}

struct SelectionItem
{
    string display;
    string description;
    string detail;
}

int displaySelectionList(T)(T[] items, string delegate(T, ulong) formatter = null)
{
    if (items.length == 0)
    {
        return 0;
    }

    foreach (ulong num; 1 .. items.length + 1)
    {
        ulong idx = items.length - num;
        if (formatter)
        {
            writeln(formatter(items[idx], num));
        }
        else
        {
            writeln(format("[%d] %s", num, items[idx].to!string));
        }
    }
    writeln("");

    return cast(int) items.length;
}

string countdownFormatter(string item, ulong num)
{
    import terminal.colors;

    string numberPart = successText("[") ~ num.to!string ~ successText("]");
    return numberPart ~ " " ~ packageName(item);
}

int mapSelectionIndex(int selection, int totalItems)
{
    if (selection <= 0 || selection > totalItems)
    {
        return -1;
    }
    return totalItems - selection;
}

ProgressCallback createProgressCallback(SpinnerContext ctx, Spinner spinner = null)
{
    return (string msg) {
        if (!ctx.noSpinner && !ctx.verbose && spinner !is null)
        {
            spinner.update(msg);
        }
        else if (ctx.verbose)
        {
            writeln(msg);
        }
    };
}

void standardErrorHandling(string action, Exception e)
{
    errorOutput(format("%s failed: %s", action, e.msg));
}

struct HostDetection
{
    static string detect()
    {
        import std.file : exists, readText;
        import std.process : environment;

        string hostname = "localhost";
        try
        {
            if (exists("/etc/hostname"))
            {
                hostname = readText("/etc/hostname").strip();
            }
        }
        catch (Exception)
        {
            hostname = environment.get("HOSTNAME", "localhost");
        }
        return hostname;
    }
}

int executeWithReturnCode(int delegate() operation, string successMsg = "", string errorMsg = "")
{
    try
    {
        int result = operation();
        if (result == 0 && successMsg.length > 0)
        {
            successOutput(successMsg);
        }
        return result;
    }
    catch (Exception e)
    {
        if (errorMsg.length > 0)
        {
            errorOutput(errorMsg ~ ": " ~ e.msg);
        }
        else
        {
            standardErrorHandling("Operation", e);
        }
        return 1;
    }
}
