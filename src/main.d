module main;

import std.stdio;
import std.stdio : writeln;
import std.string : strip;
import terminal.cli; // single entry point
import utils.sh;

int main(string[] args)
{
    return run(args[1 .. $]);
}
