using System;
using Newtonsoft.Json.Linq;

namespace UnityCliConnector
{
    /// <summary>
    /// Thrown when a parameter is present but cannot be parsed as the expected
    /// type. The router converts it into an invalid_param error response.
    /// </summary>
    public class ToolParamException : Exception
    {
        public string Param { get; }

        public ToolParamException(string param, string message) : base(message)
        {
            Param = param;
        }
    }

    public class ToolParams
    {
        private readonly JObject _params;

        public ToolParams(JObject @params)
        {
            _params = @params ?? throw new ArgumentNullException(nameof(@params));
        }

        public Result<string> GetRequired(string key, string errorMessage = null)
        {
            var value = GetString(key);
            if (string.IsNullOrEmpty(value))
                return Result<string>.Error(errorMessage ?? $"'{key}' parameter is required.");
            return Result<string>.Success(value);
        }

        public string Get(string key, string defaultValue = null)
        {
            return GetString(key) ?? defaultValue;
        }

        // Absent keys return the default; present-but-unparseable values throw
        // ToolParamException instead of being silently swallowed.
        public int? GetInt(string key, int? defaultValue = null)
        {
            var str = GetString(key);
            if (string.IsNullOrEmpty(str)) return defaultValue;
            if (int.TryParse(str, out var result)) return result;
            throw new ToolParamException(key, $"'{key}' must be an integer, got '{str}'");
        }

        public float? GetFloat(string key, float? defaultValue = null)
        {
            var str = GetString(key);
            if (string.IsNullOrEmpty(str)) return defaultValue;
            if (float.TryParse(str, System.Globalization.NumberStyles.Float,
                System.Globalization.CultureInfo.InvariantCulture, out var result)) return result;
            throw new ToolParamException(key, $"'{key}' must be a number, got '{str}'");
        }

        public bool GetBool(string key, bool defaultValue = false)
        {
            var token = GetToken(key);
            if (token == null || token.Type == JTokenType.Null) return defaultValue;
            var coerced = ParamCoercion.CoerceBoolNullable(token);
            if (coerced.HasValue) return coerced.Value;
            var str = token.ToString().Trim();
            if (str.Length == 0) return defaultValue;
            throw new ToolParamException(key, $"'{key}' must be a boolean, got '{str}'");
        }

        public JToken GetRaw(string key)
        {
            return GetToken(key);
        }

        private JToken GetToken(string key)
        {
            return _params[key];
        }

        private string GetString(string key)
        {
            return GetToken(key)?.ToString();
        }
    }

    public class Result<T>
    {
        public bool IsSuccess { get; }
        public T Value { get; }
        public string ErrorMessage { get; }

        private Result(bool isSuccess, T value, string errorMessage)
        {
            IsSuccess = isSuccess;
            Value = value;
            ErrorMessage = errorMessage;
        }

        public static Result<T> Success(T value) => new Result<T>(true, value, null);
        public static Result<T> Error(string errorMessage) => new Result<T>(false, default, errorMessage);
    }
}
