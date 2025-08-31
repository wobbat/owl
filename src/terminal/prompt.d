module terminal.prompt;

import std.stdio;
import std.string;
import std.algorithm;
import terminal.colors;

/// Prompt user for yes/no confirmation
bool confirmYesNo(string question, bool defaultYes = true)
{
    string prompt;
    if (defaultYes)
    {
        prompt = question ~ " [Y/n]: ";
    }
    else
    {
        prompt = question ~ " [y/N]: ";
    }

    write(prompt);
    stdout.flush();

    string response = readln().strip().toLower();

    if (response.empty)
    {
        return defaultYes;
    }

    return response == "y" || response == "yes";
}

/// Prompt user to select from a numbered list
int promptSelection(int maxNum)
{
    write(successText("[Enter number:] "));
    stdout.flush();

    try
    {
        string input = readln().strip();
        if (input.empty)
            return 0;

        import std.conv : to;

        int selection = input.to!int();

        if (selection >= 1 && selection <= maxNum)
        {
            return selection;
        }
        else
        {
            return 0;
        }
    }
    catch (Exception)
    {
        return 0;
    }
}

/// Prompt user for text input
string promptText(string question)
{
    write(question ~ " ");
    stdout.flush();
    return readln().strip();
}

import std.format;
