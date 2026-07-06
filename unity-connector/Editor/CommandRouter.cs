using System;
using System.Threading;
using System.Threading.Tasks;
using Newtonsoft.Json.Linq;
using UnityEngine;

namespace UnityCliConnector
{
    /// <summary>
    /// Routes incoming command requests to the appropriate tool handler.
    /// All requests are serialized through a single queue to prevent
    /// race conditions when multiple CLI agents access the same Unity instance.
    /// </summary>
    public static class CommandRouter
    {
        static readonly SemaphoreSlim s_Lock = new(1, 1);

        public static async Task<object> Dispatch(string command, JObject parameters)
        {
            await s_Lock.WaitAsync();
            try
            {
                return await DispatchInternal(command, parameters);
            }
            finally
            {
                s_Lock.Release();
            }
        }

        static async Task<object> DispatchInternal(string command, JObject parameters)
        {
            if (command == "list")
                return new SuccessResponse("Available tools", ToolDiscovery.GetToolSchemas());

            var tool = ToolDiscovery.Find(command);
            if (tool == null)
                return new ErrorResponse($"Unknown command: {command}", new { code = "unknown_command" });

            parameters ??= new JObject();
            ApplyPositionalArgs(tool, parameters);

            foreach (var schema in tool.Schema)
            {
                if (!schema.Required || HasValue(parameters, schema.Name)) continue;
                return new ErrorResponse($"'{schema.Name}' parameter is required",
                    new { code = "missing_param", param = schema.Name });
            }

            try
            {
                var result = tool.Handler.Invoke(null, new object[] { parameters });

                if (result is Task<object> asyncTask)
                    return await asyncTask;

                if (result is Task task)
                {
                    await task;
                    return new SuccessResponse($"{command} completed");
                }

                return result ?? new SuccessResponse($"{command} completed");
            }
            catch (Exception ex)
            {
                var inner = ex.InnerException ?? ex;
                if (inner is ToolParamException tpe)
                    return new ErrorResponse(tpe.Message, new { code = "invalid_param", param = tpe.Param });
                Debug.LogException(inner);
                return new ErrorResponse($"{command} failed: {inner.Message}");
            }
        }

        /// <summary>
        /// Copies positional args (params.args[i]) into named parameters whose
        /// schema declares Position = i, so positionals pass Required validation
        /// and tools no longer need hand-rolled args fallbacks.
        /// </summary>
        static void ApplyPositionalArgs(ToolDiscovery.ToolInfo tool, JObject parameters)
        {
            if (parameters["args"] is not JArray args || args.Count == 0) return;

            foreach (var schema in tool.Schema)
            {
                if (schema.Position < 0 || schema.Position >= args.Count) continue;
                if (HasValue(parameters, schema.Name)) continue;
                parameters[schema.Name] = args[schema.Position];
            }
        }

        static bool HasValue(JObject parameters, string name)
        {
            var token = parameters[name];
            if (token == null || token.Type == JTokenType.Null) return false;
            if (token.Type == JTokenType.String && string.IsNullOrEmpty(token.Value<string>())) return false;
            return true;
        }
    }
}
