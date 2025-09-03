module utils.selection;

import std.algorithm;
import std.array;
import std.conv;
import std.format;
import std.stdio;
import std.string;
import terminal.colors;
import terminal.prompt;
import terminal.ui;

int displayCountdownSelection(T)(T[] items, string delegate(T, size_t) formatter)
{
    if (items.length == 0)
    {
        return 0;
    }

    foreach (size_t i; 0 .. items.length)
    {
        writeln(formatter(items[i], i + 1));
    }
    writeln("");

    return cast(int) items.length;
}

string packageSelectionFormatter(string item, size_t num)
{
    string numberPart = successText("[") ~ num.to!string ~ successText("]");
    return numberPart ~ " " ~ packageName(item);
}

string fileSelectionFormatter(string file, size_t num, string homeDir = "")
{
    string friendly = homeDir.length > 0 ? file.replace(homeDir, "~") : file;
    string numberPart = successText("[") ~ num.to!string ~ successText("]");
    string fileName = packageName(friendly.canFind('/') ? friendly.split('/')[$ - 1] : friendly);
    string pathPart = "(" ~ highlight(friendly) ~ ")";
    string countPart = brackets("config", Repository);
    return numberPart ~ " " ~ fileName ~ " " ~ pathPart ~ " " ~ countPart;
}

int mapCountdownSelection(int selection, int totalItems)
{
    if (selection <= 0 || selection > totalItems)
    {
        return -1;
    }
    return selection - 1;
}

struct SelectionResult(T)
{
    bool valid;
    T item;
    int index;
}

SelectionResult!T handleSelection(T)(T[] items, string prompt = "")
{
    if (items.length == 0)
    {
        return SelectionResult!T(false, T.init, -1);
    }

    int selection = promptSelection(cast(int) items.length);
    int mappedIndex = mapCountdownSelection(selection, cast(int) items.length);

    if (mappedIndex < 0)
    {
        return SelectionResult!T(false, T.init, -1);
    }

    return SelectionResult!T(true, items[mappedIndex], mappedIndex);
}
