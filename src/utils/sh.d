module utils.sh;

import std.process;
import std.stdio;
import std.array;

/// Run a shell command given as one string. Returns combined stdout+stderr.
string run(string cmd)
{
    // Merge stderr into stdout so we only read one stream
    auto p = pipeProcess(["/bin/sh", "-c", cmd], Redirect.stdout | Redirect.stderrToStdout);

    scope (exit)
    {
        // Close our read end before waiting to avoid hanging on some programs
        p.stdout.close();
        wait(p.pid);
    }

    auto buf = appender!string();
    foreach (line; p.stdout.byLine())
    {
        buf.put(line);
        buf.put('\n');
    }
    return buf.data;
}
