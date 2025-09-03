module packages.aur;

import std.algorithm;
import std.array;
import core.time;
import std.conv;
import std.json;
import std.net.curl;
import std.string;
import std.uri;
import packages.types;

struct AurInfo {
    string name;
    string ver;
    string description;
    string url;
}

private string buildQuery(string[2][] params) {
    if (params.length == 0) {
        return "";
    }

    string[] queryParts;
    foreach (param; params) {
        string key = param[0];
        string value = param[1];

        if (key == "arg") {
            // Use array format for batch info requests: arg%5B%5D=value (URL-encoded arg[])
            value = encodeComponent(value).replace("%20", "+");
            queryParts ~= "arg%5B%5D=" ~ value;
        } else {
            value = encodeComponent(value);
            queryParts ~= key ~ "=" ~ value;
        }
    }

    return "?" ~ queryParts.join("&");
}

private JSONValue tryApiEndpoint(string url) {
    auto http = HTTP();
    http.addRequestHeader("User-Agent", "owl-d/0.1");
    http.connectTimeout = dur!"seconds"(3); // Reduced from 8 to 3 seconds
    http.operationTimeout = dur!"seconds"(5); // Add operation timeout

    char[] content = get(url, http);
    return parseJSON(content.to!string);
}

JSONValue aurRpc(string endpoint, string[2][] params) {
    string query = buildQuery(params);

    // Try modern v5 path first
    string modernUrl = "https://aur.archlinux.org/rpc/v5/" ~ endpoint ~ query;
    try {
        return tryApiEndpoint(modernUrl);
    } catch (Exception) {
        // Fall back to legacy query style
        string[2][] compatParams = [["v", "5"], ["type", endpoint]];
        compatParams ~= params;

        string legacyUrl = "https://aur.archlinux.org/rpc/" ~ buildQuery(compatParams);

        try {
            return tryApiEndpoint(legacyUrl);
        } catch (Exception e) {
            throw new Exception("AUR RPC failed: all endpoints unavailable - " ~ e.msg);
        }
    }
}

SearchResult[] search(string[] terms, bool useParu = false) {
    if (terms.length == 0)
        return [];

    // Check if paru should be used and is available
    bool paruAvailable = false;
    if (useParu) {
        import packages.pacman : ensureParuAvailable;
        paruAvailable = ensureParuAvailable();
    }

    // If paru is available, use it for search
    if (paruAvailable) {
        return searchWithParu(terms);
    }

    try {
        string query = terms.join(" ");
        // Prefer broader matching: name + description
        JSONValue resp = aurRpc("search", [["by", "name-desc"], ["arg", query]]);

        SearchResult[] results;
        if ("results" in resp && resp["results"].type == JSONType.array) {
            foreach (item; resp["results"].array) {
                SearchResult result;
                result.name = item["Name"].str;
                result.ver = item["Version"].str;
                result.source = PackageSource.aur;
                result.repo = "aur";

                if ("Description" in item && item["Description"].type == JSONType.string) {
                    result.description = item["Description"].str;
                } else {
                    result.description = "";
                }

                result.installed = false;
                results ~= result;
            }
        }

        return results;
    } catch (Exception e) {
        // Tolerate AUR/network errors
        return [];
    }
}

/// Search AUR using paru
SearchResult[] searchWithParu(string[] terms) {
    if (terms.length == 0)
        return [];

    try {
        import std.process : execute;
        import std.string : splitLines;

        // Run paru with -Ss and --bottomup
        auto result = execute(["paru", "-Ss", "--bottomup"] ~ terms);
        if (result.status != 0) {
            // Fall back to API search if paru fails
            return search(terms, false);
        }

        string[] outLines = result.output.splitLines();

        // Parse collected output similar to non-paru parsing
        SearchResult[] results;
        string curName = "";
        string curVer = "";
        string curDesc = "";
        string curRepo = "";

        foreach (line; outLines) {
            if (line.length == 0)
                continue;

            // Check for paru numbered output: number repo/name version [flags]
            if (!line.startsWith(" ") && line.canFind("/") && line.canFind("aur/")) {
                // Flush previous result
                if (curName.length > 0) {
                    auto src = curRepo == "aur" ? PackageSource.aur : PackageSource.repo;
                    results ~= SearchResult(name: curName, ver: curVer, source: src,
                repo: curRepo, description: curDesc, installed: false);
                }

                // Parse paru numbered line: 1365 aur/zwm-git 0.1.13-1 [+0 ~0.00]
                import std.string : split;
                auto parts = split(line);

                if (parts.length >= 3) {
                    // Check if first part is a number (paru's internal numbering)
                    size_t startIndex = 0;
                    import std.conv : to;
                    try {
                        to!int(parts[0]); // Try to parse as number
                        startIndex = 1; // Skip the number
                    } catch (Exception) {
                        // Not a number, start from index 0
                    }

                    if (parts.length > startIndex) {
                        string repoNamePart = parts[startIndex];

                        if (repoNamePart.length > 0 && repoNamePart.canFind("/")) {
                            auto rv = split(repoNamePart, "/");
                            if (rv.length == 2) {
                                curRepo = rv[0];
                                curName = rv[1];
                            } else {
                                curRepo = "";
                                curName = repoNamePart;
                            }
                        }

                        // Version is next token if present
                        if (parts.length > startIndex + 1) {
                            curVer = parts[startIndex + 1];
                        } else {
                            curVer = "";
                        }

                        curDesc = "";
                    }
                }
            }
            // Header lines are non-indented lines containing a '/' (fallback for non-numbered format)
            else if (line.canFind("/") && line.canFind(" ") && !line.startsWith(" ") && !line.startsWith("[")) {
                // Flush previous
                if (curName.length > 0) {
                    auto src = curRepo == "aur" ? PackageSource.aur : PackageSource.repo;
                    results ~= SearchResult(name: curName, ver: curVer, source: src,
                repo: curRepo, description: curDesc, installed: false);
                }

                // Parse line like: [number] repo/name version [flags] or repo/name version [flags]
                import std.string : split;
                auto parts = split(line);

                // Check if first part is a number (from --bottomup)
                size_t startIndex = 0;
                if (parts.length > 0 && parts[0].length > 0) {
                    import std.conv : to;
                    try {
                        to!int(parts[0]); // Try to parse as number
                        startIndex = 1; // Skip the number
                    } catch (Exception) {
                        // Not a number, start from index 0
                    }
                }

                string repoName = "";
                if (parts.length > startIndex) {
                    repoName = parts[startIndex];
                }

                if (repoName.length > 0 && repoName.canFind("/")) {
                    auto rv = split(repoName, "/");
                    if (rv.length == 2) {
                        curRepo = rv[0];
                        curName = rv[1];
                    } else {
                        curRepo = "";
                        curName = repoName;
                    }
                } else {
                    curRepo = "";
                    curName = repoName;
                }

                // Version is next token if present
                if (parts.length > startIndex + 1) {
                    curVer = parts[startIndex + 1];
                } else {
                    curVer = "";
                }

                curDesc = "";
            } else if (line.startsWith("    ")) {
                // Description lines are indented
                curDesc = line.strip();
            }
        }

        if (curName.length > 0) {
            auto src = curRepo == "aur" ? PackageSource.aur : PackageSource.repo;
            results ~= SearchResult(name: curName, ver: curVer, source: src, repo: curRepo.length > 0
                    ? curRepo : "", description: curDesc, installed: false);
        }

        return results;
    } catch (Exception e) {
        // Fall back to API search if paru search fails
        return search(terms, false);
    }
}

PackageInfo info(string name) {
    auto results = infoBatch([name]);
    if (results.length > 0)
        return results[0];
    return PackageInfo();
}

PackageInfo[] infoBatch(string[] names) {
    if (names.length == 0)
        return [];

    try {
        // Build parameters for batch request
        string[2][] params;
        foreach (name; names) {
            params ~= ["arg", name];
        }

        JSONValue resp = aurRpc("info", params);
        PackageInfo[] results;

        if ("results" in resp && resp["results"].type == JSONType.array) {
            foreach (item; resp["results"].array) {
                PackageInfo info;
                info.name = item["Name"].str;
                info.ver = item["Version"].str;
                info.source = PackageSource.aur;
                info.repo = "aur";

                if ("Description" in item && item["Description"].type == JSONType.string) {
                    info.description = item["Description"].str;
                } else {
                    info.description = "";
                }

                info.installed = false;
                info.explicit = false;
                results ~= info;
            }
        }

        return results;
    } catch (Exception e) {
        // Return empty array if API call fails
        return [];
    }
}
