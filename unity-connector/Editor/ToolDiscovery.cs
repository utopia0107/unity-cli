using System;
using System.Collections.Generic;
using System.Linq;
using System.Reflection;
using UnityEditor;
using UnityEngine;

namespace UnityCliConnector
{
    /// <summary>
    /// Discovers all classes with [UnityCliTool] or [McpForUnityTool] attributes
    /// and registers their HandleCommand methods with the CommandRouter.
    /// </summary>
    [InitializeOnLoad]
    public static class ToolDiscovery
    {
        public class ToolInfo
        {
            public string Name;
            public string Description;
            public string Group;
            public MethodInfo Handler;
            public Type ParametersType;
        }

        static readonly Dictionary<string, ToolInfo> s_Tools = new();

        static ToolDiscovery()
        {
            Discover();
        }

        public static IReadOnlyDictionary<string, ToolInfo> Tools => s_Tools;

        public static void Discover()
        {
            s_Tools.Clear();

            foreach (var assembly in AppDomain.CurrentDomain.GetAssemblies())
            {
                try
                {
                    foreach (var type in assembly.GetTypes())
                    {
                        TryRegister(type);
                    }
                }
                catch (ReflectionTypeLoadException)
                {
                    // Skip assemblies that fail to load
                }
            }

            Debug.Log($"[UnityCliConnector] Discovered {s_Tools.Count} tools");
        }

        static void TryRegister(Type type)
        {
            if (type.IsAbstract == false && type.IsClass == false) return;

            string name = null;
            string description = "";
            string group = "";

            var cliAttr = type.GetCustomAttribute<UnityCliToolAttribute>();
            if (cliAttr == null) return;

            name = cliAttr.Name ?? StringCaseUtility.ToSnakeCase(type.Name);
            description = cliAttr.Description;
            group = cliAttr.Group;

            if (name == null) return;

            var handler = type.GetMethod("HandleCommand",
                BindingFlags.Public | BindingFlags.Static,
                null,
                new[] { typeof(Newtonsoft.Json.Linq.JObject) },
                null);

            if (handler == null)
            {
                Debug.LogWarning($"[UnityCliConnector] {type.Name} has tool attribute but no HandleCommand(JObject) method");
                return;
            }

            var paramsType = type.GetNestedType("Parameters");

            s_Tools[name] = new ToolInfo
            {
                Name = name,
                Description = description,
                Group = group,
                Handler = handler,
                ParametersType = paramsType,
            };
        }

        public static List<object> GetToolSchemas()
        {
            return s_Tools.Values.Select(t => new
            {
                name = t.Name,
                description = t.Description,
                group = t.Group,
                parameters = GetParameterSchema(t.ParametersType),
            }).Cast<object>().ToList();
        }

        static List<object> GetParameterSchema(Type paramsType)
        {
            if (paramsType == null) return new List<object>();

            return paramsType.GetProperties()
                .Select(p =>
                {
                    var attr = p.GetCustomAttribute<ToolParameterAttribute>();
                    return new
                    {
                        name = StringCaseUtility.ToSnakeCase(p.Name),
                        type = p.PropertyType.Name,
                        description = attr?.Description ?? "",
                        required = attr?.Required ?? false,
                    };
                })
                .Cast<object>()
                .ToList();
        }
    }
}
