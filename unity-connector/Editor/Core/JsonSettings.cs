using Newtonsoft.Json;
using Newtonsoft.Json.Serialization;

namespace UnityCliConnector
{
    /// <summary>
    /// Shared serializer settings: every response payload uses snake_case keys.
    /// Dictionary keys are NOT converted — exec serializes user-code field
    /// names verbatim. Note that JObject/JToken payloads bypass the naming
    /// strategy; tools must build those with snake_case keys themselves.
    /// </summary>
    public static class JsonSettings
    {
        public static readonly JsonSerializerSettings Settings = new JsonSerializerSettings
        {
            ContractResolver = new DefaultContractResolver
            {
                NamingStrategy = new SnakeCaseNamingStrategy(processDictionaryKeys: false, overrideSpecifiedNames: false),
            },
        };
    }
}
