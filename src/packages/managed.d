module packages.managed;

import std.algorithm;
import std.array;
import std.conv;
import std.file;
import std.json;
import std.path;
import std.stdio;
import std.string;
import config.paths;

struct PackageMetadata
{
    string firstManaged;
    string lastSeen;
    string installedVersion;
    bool autoInstalled;
}

struct ManagedState
{
    string schemaVersion;
    PackageMetadata[string] packages;
    string[] protectedPackages;
}

/// Get path to managed packages state file
string managedStatePath()
{
    return owlManagedState();
}

/// Default protected packages (critical system packages)
string[] defaultProtectedPackages()
{
    return [
        "systemd", "linux-firmware", "grep", "dbus-broker", "sed", "gawk",
        "pacman-contrib", "grub", "systemd-sysvcompat", "ca-certificates",
        "fish", "binutils", "zsh", "polkit", "filesystem",
        "archlinux-keyring", "networkmanager", "util-linux", "iwd", "refind",
        "sudo", "bash", "base-devel", "linux", "wpa_supplicant", "coreutils",
        "pacman", "dbus", "gcc-libs", "bootctl", "linux-headers", "dhcpcd",
        "glibc", "base"
    ];
}

/// Read managed packages state
ManagedState readManagedState()
{
    string path = managedStatePath();
    ManagedState state;
    state.schemaVersion = "1.0";
    state.protectedPackages = defaultProtectedPackages();

    if (!exists(path))
    {
        return state;
    }

    try
    {
        string content = readText(path);
        JSONValue json = parseJSON(content);

        if ("schema_version" in json)
        {
            state.schemaVersion = json["schema_version"].str;
        }

        if ("packages" in json)
        {
            foreach (pkg, metadata; json["packages"].object)
            {
                PackageMetadata meta;
                if ("first_managed" in metadata)
                {
                    meta.firstManaged = metadata["first_managed"].str;
                }
                if ("last_seen" in metadata)
                {
                    meta.lastSeen = metadata["last_seen"].str;
                }
                if ("installed_version" in metadata)
                {
                    meta.installedVersion = metadata["installed_version"].str;
                }
                if ("auto_installed" in metadata)
                {
                    meta.autoInstalled = metadata["auto_installed"].type == JSONType.true_;
                }
                state.packages[pkg] = meta;
            }
        }

        if ("protected_packages" in json)
        {
            state.protectedPackages = [];
            foreach (item; json["protected_packages"].array)
            {
                state.protectedPackages ~= item.str;
            }
        }

        return state;
    }
    catch (Exception e)
    {
        // If parsing fails, return default state
        return state;
    }
}

/// Write managed packages state
void writeManagedState(ManagedState state)
{
    string path = managedStatePath();
    mkdirRecurse(dirName(path));

    JSONValue json = JSONValue();
    json["schema_version"] = JSONValue(state.schemaVersion);

    JSONValue packages = JSONValue();
    foreach (pkg, metadata; state.packages)
    {
        JSONValue meta = JSONValue();
        meta["first_managed"] = JSONValue(metadata.firstManaged);
        meta["last_seen"] = JSONValue(metadata.lastSeen);
        meta["installed_version"] = JSONValue(metadata.installedVersion);
        meta["auto_installed"] = JSONValue(metadata.autoInstalled);
        packages[pkg] = meta;
    }
    json["packages"] = packages;

    JSONValue protectedArray = JSONValue();
    protectedArray.array = [];
    foreach (pkg; state.protectedPackages)
    {
        protectedArray.array ~= JSONValue(pkg);
    }
    json["protected_packages"] = protectedArray;

    std.file.write(path, json.toPrettyString());
}

/// Get list of managed packages
string[] readManagedPackages()
{
    auto state = readManagedState();
    return state.packages.keys;
}

/// Get list of protected packages
string[] readProtectedPackages()
{
    auto state = readManagedState();
    return state.protectedPackages;
}

/// Add package to managed state
void addManagedPackage(string packageName, string packageVersion = "", bool autoInstalled = false)
{
    auto state = readManagedState();

    if (packageName !in state.packages)
    {
        PackageMetadata meta;
        meta.firstManaged = ""; // TODO: Set current timestamp
        meta.lastSeen = ""; // TODO: Set current timestamp
        meta.installedVersion = packageVersion;
        meta.autoInstalled = autoInstalled;
        state.packages[packageName] = meta;
    }
    else
    {
        // Update existing entry
        state.packages[packageName].lastSeen = ""; // TODO: Set current timestamp
        state.packages[packageName].installedVersion = packageVersion;
    }

    writeManagedState(state);
}

/// Remove package from managed state
void removeManagedPackage(string packageName)
{
    auto state = readManagedState();
    state.packages.remove(packageName);
    writeManagedState(state);
}

/// Update managed packages list
void updateManagedPackages(string[] packageNames)
{
    auto state = readManagedState();

    // Clear existing packages and add new ones
    state.packages.clear();
    foreach (pkg; packageNames)
    {
        PackageMetadata meta;
        meta.firstManaged = ""; // TODO: Set current timestamp
        meta.lastSeen = ""; // TODO: Set current timestamp
        meta.installedVersion = "";
        meta.autoInstalled = false;
        state.packages[pkg] = meta;
    }

    writeManagedState(state);
}
