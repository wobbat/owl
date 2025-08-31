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

struct AurInfo
{
    string name;
    string ver;
    string description;
    string url;
}

private string buildQuery(string[2][] params)
{
    if (params.length == 0)
    {
        return "";
    }

    string[] queryParts;
    foreach (param; params)
    {
        string key = param[0];
        string value = param[1];

        if (key == "arg")
        {
            // Use array format for batch info requests: arg%5B%5D=value (URL-encoded arg[])
            value = encodeComponent(value).replace("%20", "+");
            queryParts ~= "arg%5B%5D=" ~ value;
        }
        else
        {
            value = encodeComponent(value);
            queryParts ~= key ~ "=" ~ value;
        }
    }

    return "?" ~ queryParts.join("&");
}

private JSONValue tryApiEndpoint(string url)
{
    auto http = HTTP();
    http.addRequestHeader("User-Agent", "owl-d/0.1");
    http.connectTimeout = dur!"seconds"(3); // Reduced from 8 to 3 seconds
    http.operationTimeout = dur!"seconds"(5); // Add operation timeout

    char[] content = get(url, http);
    return parseJSON(content.to!string);
}

JSONValue aurRpc(string endpoint, string[2][] params)
{
    string query = buildQuery(params);

    // Try modern v5 path first
    string modernUrl = "https://aur.archlinux.org/rpc/v5/" ~ endpoint ~ query;
    try
    {
        return tryApiEndpoint(modernUrl);
    }
    catch (Exception)
    {
        // Fall back to legacy query style
        string[2][] compatParams = [["v", "5"], ["type", endpoint]];
        compatParams ~= params;

        string legacyUrl = "https://aur.archlinux.org/rpc/" ~ buildQuery(compatParams);

        try
        {
            return tryApiEndpoint(legacyUrl);
        }
        catch (Exception e)
        {
            throw new Exception("AUR RPC failed: all endpoints unavailable - " ~ e.msg);
        }
    }
}

SearchResult[] search(string[] terms)
{
    if (terms.length == 0)
        return [];

    try
    {
        string query = terms.join(" ");
        // Prefer broader matching: name + description
        JSONValue resp = aurRpc("search", [["by", "name-desc"], ["arg", query]]);

        SearchResult[] results;
        if ("results" in resp && resp["results"].type == JSONType.array)
        {
            foreach (item; resp["results"].array)
            {
                SearchResult result;
                result.name = item["Name"].str;
                result.ver = item["Version"].str;
                result.source = PackageSource.aur;
                result.repo = "aur";

                if ("Description" in item && item["Description"].type == JSONType.string)
                {
                    result.description = item["Description"].str;
                }
                else
                {
                    result.description = "";
                }

                result.installed = false;
                results ~= result;
            }
        }

        return results;
    }
    catch (Exception e)
    {
        // Tolerate AUR/network errors
        return [];
    }
}

PackageInfo info(string name)
{
    auto results = infoBatch([name]);
    if (results.length > 0)
        return results[0];
    return PackageInfo();
}

PackageInfo[] infoBatch(string[] names)
{
    if (names.length == 0)
        return [];

    try
    {
        // Build parameters for batch request
        string[2][] params;
        foreach (name; names)
        {
            params ~= ["arg", name];
        }

        JSONValue resp = aurRpc("info", params);
        PackageInfo[] results;

        if ("results" in resp && resp["results"].type == JSONType.array)
        {
            foreach (item; resp["results"].array)
            {
                PackageInfo info;
                info.name = item["Name"].str;
                info.ver = item["Version"].str;
                info.source = PackageSource.aur;
                info.repo = "aur";

                if ("Description" in item && item["Description"].type == JSONType.string)
                {
                    info.description = item["Description"].str;
                }
                else
                {
                    info.description = "";
                }

                info.installed = false;
                info.explicit = false;
                results ~= info;
            }
        }

        return results;
    }
    catch (Exception e)
    {
        // Return empty array if API call fails
        return [];
    }
}
