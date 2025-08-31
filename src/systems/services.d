module systems.services;

import std.algorithm;
import std.array;
import std.process;
import std.stdio;
import std.string;
import std.conv;
import terminal.ui;
import terminal.colors;

struct ServiceResult
{
    bool changed;
    string[] enabledServices;
    string[] startedServices;
    string[] failedServices;
}

/// Check if systemctl is available
bool isSystemdAvailable()
{
    try
    {
        auto result = execute(["systemctl", "--version"]);
        return result.status == 0;
    }
    catch (Exception)
    {
        return false;
    }
}

/// Check if a service is enabled
bool isServiceEnabled(string serviceName)
{
    try
    {
        auto result = execute(["systemctl", "is-enabled", serviceName]);
        return result.status == 0;
    }
    catch (Exception)
    {
        return false;
    }
}

/// Check if a service is active/running
bool isServiceActive(string serviceName)
{
    try
    {
        auto result = execute(["systemctl", "is-active", serviceName]);
        return result.status == 0;
    }
    catch (Exception)
    {
        return false;
    }
}

/// Enable a systemd service
bool enableService(string serviceName)
{
    try
    {
        auto result = execute(["sudo", "systemctl", "enable", serviceName]);
        return result.status == 0;
    }
    catch (Exception)
    {
        return false;
    }
}

/// Start a systemd service
bool startService(string serviceName)
{
    try
    {
        auto result = execute(["sudo", "systemctl", "start", serviceName]);
        return result.status == 0;
    }
    catch (Exception)
    {
        return false;
    }
}

/// Ensure services are configured (enabled and started)
ServiceResult ensureServicesConfigured(string[] services)
{
    ServiceResult result;

    if (!isSystemdAvailable())
    {
        writeln(warningText("Warning: systemd not available, skipping service management"));
        return result;
    }

    foreach (serviceName; services)
    {
        bool needsEnable = !isServiceEnabled(serviceName);
        bool needsStart = !isServiceActive(serviceName);

        if (needsEnable)
        {
            if (enableService(serviceName))
            {
                result.enabledServices ~= serviceName;
                result.changed = true;
            }
            else
            {
                result.failedServices ~= serviceName;
                continue;
            }
        }

        if (needsStart)
        {
            if (startService(serviceName))
            {
                result.startedServices ~= serviceName;
                result.changed = true;
            }
            else
            {
                result.failedServices ~= serviceName;
            }
        }
    }

    return result;
}
