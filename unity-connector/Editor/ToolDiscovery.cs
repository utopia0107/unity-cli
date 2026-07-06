using System;
using System.Collections.Generic;
using System.Linq;
using System.Reflection;

namespace UnityCliConnector
{
    /// <summary>
    /// Discovers [UnityCliTool] handlers via reflection. The AppDomain scan runs
    /// once per domain and is cached; loading a new assembly (e.g. exec) or a
    /// domain reload invalidates the cache automatically.
    /// </summary>
    public static class ToolDiscovery
    {
        public class ParamSchema
        {
            public string Name;
            public string Type;
            public string Description;
            public bool Required;
            public int Position = -1; // >= 0: may be supplied via the positional args array
        }

        public class ToolInfo
        {
            public string Name;
            public string Description;
            public string Group;
            public MethodInfo Handler;
            public List<ParamSchema> Schema;
        }

        static readonly object s_CacheLock = new object();
        static Dictionary<string, ToolInfo> s_Cache;

        static ToolDiscovery()
        {
            AppDomain.CurrentDomain.AssemblyLoad += (_, __) =>
            {
                lock (s_CacheLock) s_Cache = null;
            };
        }

        public static ToolInfo Find(string command)
        {
            var cache = GetCache();
            return cache.TryGetValue(command, out var info) ? info : null;
        }

        public static MethodInfo FindHandler(string command) => Find(command)?.Handler;

        public static List<object> GetToolSchemas()
        {
            return GetCache().Values
                .OrderBy(t => t.Name, StringComparer.Ordinal)
                .Select(t => (object)new
                {
                    name = t.Name,
                    description = t.Description,
                    group = t.Group,
                    parameters = t.Schema.Select(s => (object)new
                    {
                        name = s.Name,
                        type = s.Type,
                        description = s.Description,
                        required = s.Required,
                        position = s.Position,
                    }).ToList(),
                })
                .ToList();
        }

        static Dictionary<string, ToolInfo> GetCache()
        {
            lock (s_CacheLock)
            {
                s_Cache ??= BuildCache();
                return s_Cache;
            }
        }

        static Dictionary<string, ToolInfo> BuildCache()
        {
            var tools = new Dictionary<string, ToolInfo>();

            foreach (var assembly in AppDomain.CurrentDomain.GetAssemblies())
            {
                Type[] types;
                try { types = assembly.GetTypes(); }
                catch (ReflectionTypeLoadException) { continue; }

                foreach (var type in types)
                {
                    if (type.IsClass == false) continue;
                    var attr = type.GetCustomAttribute<UnityCliToolAttribute>();
                    if (attr == null) continue;

                    var name = attr.Name ?? StringCaseUtility.ToSnakeCase(type.Name);

                    var method = type.GetMethod("HandleCommand",
                        BindingFlags.Public | BindingFlags.Static, null,
                        new[] { typeof(Newtonsoft.Json.Linq.JObject) }, null);
                    if (method == null) continue;

                    if (tools.TryGetValue(name, out var existing))
                    {
                        UnityEngine.Debug.LogError(
                            $"[UnityCliConnector] Duplicate tool name '{name}': " +
                            $"{existing.Handler.DeclaringType.FullName} and {type.FullName}. " +
                            $"Rename one or remove the duplicate. Using first found.");
                        continue;
                    }

                    tools[name] = new ToolInfo
                    {
                        Name = name,
                        Description = attr.Description ?? "",
                        Group = attr.Group ?? "",
                        Handler = method,
                        Schema = GetParameterSchema(type.GetNestedType("Parameters")),
                    };
                }
            }

            return tools;
        }

        public static List<ParamSchema> GetParameterSchema(Type paramsType)
        {
            if (paramsType == null) return new List<ParamSchema>();

            return paramsType.GetProperties()
                .Select(p =>
                {
                    var attr = p.GetCustomAttribute<ToolParameterAttribute>();
                    return new ParamSchema
                    {
                        Name = attr?.Name ?? StringCaseUtility.ToSnakeCase(p.Name),
                        Type = p.PropertyType.Name,
                        Description = attr?.Description ?? "",
                        Required = attr?.Required ?? false,
                        Position = attr?.Position ?? -1,
                    };
                })
                .ToList();
        }
    }
}
