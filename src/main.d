module main;

import terminal.cli;

int main(string[] args)
{
    return run(args[1 .. $]);
}
