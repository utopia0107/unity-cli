using System;
using System.Collections;
using System.Collections.Generic;
using System.Diagnostics;
using System.IO;
using System.Linq;
using System.Reflection;
using System.Security.Cryptography;
using System.Text;
using System.Threading.Tasks;
using Newtonsoft.Json.Linq;
using UnityEditor;
using UnityEngine;

namespace UnityCliConnector.Tools
{
    [UnityCliTool(Name = "exec", Description = "Execute arbitrary C# code at runtime. Full access to Unity and all loaded assemblies.")]
    public static class ExecuteCsharp
    {
        private const int DefaultTimeoutSec = 30;

        private static readonly string[] DefaultUsings =
        {
            "System",
            "System.Collections.Generic",
            "System.IO",
            "System.Linq",
            "System.Reflection",
            "System.Threading.Tasks",
            "UnityEngine",
            "UnityEngine.SceneManagement",
            "UnityEditor",
            "UnityEditor.SceneManagement",
            "UnityEditorInternal",
        };

        // Compiled snippets keyed by source hash. Skips csc for repeated
        // identical code (the common agent pattern) and bounds the
        // Assembly.Load leak to one assembly per unique snippet per domain.
        // Main-thread access only (phases 1 and 3 run on the main thread).
        private static readonly Dictionary<string, MethodInfo> s_Cache = new Dictionary<string, MethodInfo>();

        public class Parameters
        {
            [ToolParameter("C# code to execute. Use 'return' for output.", Required = true, Position = 0)]
            public string Code { get; set; }

            [ToolParameter("Additional using directives (comma-separated, e.g. Unity.Entities,Unity.Mathematics)")]
            public string[] Usings { get; set; }

            [ToolParameter("Path to csc compiler (csc.dll or csc.exe). Auto-detected if omitted.")]
            public string Csc { get; set; }

            [ToolParameter("Path to dotnet runtime. Auto-detected if omitted.")]
            public string Dotnet { get; set; }

            [ToolParameter("Compile timeout in seconds (default 30). Does not bound the execution phase.")]
            public int TimeoutSec { get; set; }
        }

        public static async Task<object> HandleCommand(JObject parameters)
        {
            // Phase 1 — main thread: parse params, build source, capture every
            // Unity-API-dependent value needed by the background compile.
            var p = new ToolParams(parameters);
            var code = p.Get("code");
            if (string.IsNullOrEmpty(code))
                return new ErrorResponse("'code' required");

            var usingsToken = p.GetRaw("usings");
            var extraUsings = new List<string>();
            if (usingsToken != null)
            {
                if (usingsToken.Type == JTokenType.Array)
                    extraUsings.AddRange(usingsToken.ToObject<string[]>());
                else
                    extraUsings.AddRange(usingsToken.ToString().Split(','));
            }

            var timeoutSec = p.GetInt("timeout_sec", DefaultTimeoutSec).Value;
            if (timeoutSec <= 0) timeoutSec = DefaultTimeoutSec;

            var source = BuildSource(code, extraUsings, out var headerLines);
            var hash = Sha256(source);
            var cached = s_Cache.TryGetValue(hash, out var method);

            if (!cached)
            {
                var csc = FindCsc(p.Get("csc"));
                if (csc == null)
                    return new ErrorResponse(
                        "Cannot find csc compiler under: " + EditorApplication.applicationContentsPath +
                        "\nSpecify the path manually with --csc <path-to-csc.dll-or-csc.exe>");

                string dotnet = null;
                if (csc.EndsWith(".dll"))
                {
                    dotnet = FindDotnet(p.Get("dotnet"));
                    if (dotnet == null)
                        return new ErrorResponse(
                            "Cannot find dotnet runtime under: " + EditorApplication.applicationContentsPath +
                            "\nSpecify the path manually with --dotnet <path>");
                }

                var references = CollectReferences();

                // Phase 2 — thread pool: the external csc process runs without
                // blocking the editor; EditorApplication.update keeps pumping.
                var compile = await Task.Run(() => Compile(source, references, csc, dotnet, timeoutSec));

                // Phase 3 — back on the main thread (Unity synchronization context).
                if (compile.ErrorMessage != null)
                {
                    if (compile.Diagnostics != null && compile.Diagnostics.Count > 0)
                    {
                        var errors = CscOutputParser.ToUserErrors(compile.Diagnostics, headerLines);
                        var first = compile.Diagnostics.FirstOrDefault(d => d.Severity == "error");
                        var summary = first != null ? $"{first.Code}: {first.Message}" : compile.ErrorMessage;
                        return new ErrorResponse($"Compile error: {summary}", new { errors });
                    }
                    return new ErrorResponse(compile.ErrorMessage);
                }

                var compiled = Assembly.Load(compile.AssemblyBytes);
                method = compiled.GetType("__CliDynamic")?.GetMethod("Execute");
                if (method == null)
                    return new ErrorResponse("Internal error: compiled type or method not found.");
                s_Cache[hash] = method;
            }

            object result;
            try
            {
                result = method.Invoke(null, null);
            }
            catch (TargetInvocationException tie)
            {
                var inner = tie.InnerException ?? tie;
                return new ErrorResponse($"Runtime error: {inner.GetType().Name}: {inner.Message}");
            }
            return new SuccessResponse(cached ? "OK (cached)" : "OK", Serialize(result, 0));
        }

        private static string BuildSource(string code, List<string> extraUsings, out int headerLines)
        {
            var sb = new StringBuilder();
            foreach (var u in DefaultUsings)
                sb.AppendLine($"using {u};");
            foreach (var u in extraUsings)
                sb.AppendLine($"using {u};");

            sb.AppendLine();
            sb.AppendLine("public static class __CliDynamic {");
            sb.AppendLine("  public static object Execute() {");
            sb.AppendLine(code);
            sb.AppendLine("  }");
            sb.AppendLine("}");

            // usings + blank + class line + method line precede the user's code.
            headerLines = DefaultUsings.Length + extraUsings.Count + 3;
            return sb.ToString();
        }

        private static string Sha256(string text)
        {
            using var sha = SHA256.Create();
            return BitConverter.ToString(sha.ComputeHash(Encoding.UTF8.GetBytes(text))).Replace("-", "");
        }

        private static List<string> CollectReferences()
        {
            var references = new List<string>();
            var added = new HashSet<string>();
            foreach (var asm in AppDomain.CurrentDomain.GetAssemblies())
            {
                try
                {
                    if (asm.IsDynamic || string.IsNullOrEmpty(asm.Location)) continue;
                    if (!added.Add(asm.GetName().Name)) continue;
                    references.Add(asm.Location);
                }
                catch { }
            }
            return references;
        }

        private class CompileResult
        {
            public byte[] AssemblyBytes;
            public string ErrorMessage;
            public List<CscOutputParser.Diagnostic> Diagnostics;
        }

        // Runs on a thread-pool thread: no Unity API access allowed here.
        private static CompileResult Compile(string source, List<string> references, string csc, string dotnet, int timeoutSec)
        {
            var utf8 = new UTF8Encoding(false);
            var tmpDir = Path.Combine(Path.GetTempPath(), "unity-cli-exec");
            Directory.CreateDirectory(tmpDir);

            var id = Guid.NewGuid().ToString("N").Substring(0, 8);
            var srcFile = Path.Combine(tmpDir, $"{id}.cs");
            var outFile = Path.Combine(tmpDir, $"{id}.dll");
            var rspFile = Path.Combine(tmpDir, $"{id}.rsp");

            try
            {
                File.WriteAllText(srcFile, source, utf8);

                var rsp = new StringBuilder();
                rsp.AppendLine("-target:library");
                rsp.AppendLine($"-out:\"{outFile}\"");
                rsp.AppendLine("-nologo");
                rsp.AppendLine("-nowarn:0105,1701,1702");
                rsp.AppendLine("-langversion:latest");
                rsp.AppendLine($"\"{srcFile}\"");
                foreach (var reference in references)
                    rsp.AppendLine($"-r:\"{reference}\"");
                File.WriteAllText(rspFile, rsp.ToString(), utf8);

                var rspArg = $"@\"{rspFile}\"";
                string exe, args;
                if (dotnet != null)
                {
                    exe = dotnet;
                    args = $"exec \"{csc}\" {rspArg}";
                }
                else
                {
                    exe = csc;
                    args = rspArg;
                }

                var psi = new ProcessStartInfo
                {
                    FileName = exe,
                    Arguments = args,
                    UseShellExecute = false,
                    RedirectStandardOutput = true,
                    RedirectStandardError = true,
                    CreateNoWindow = true,
                    StandardOutputEncoding = Encoding.UTF8,
                    StandardErrorEncoding = Encoding.UTF8,
                };

                using (var proc = Process.Start(psi))
                {
                    // Drain both streams concurrently — sequential ReadToEnd can
                    // deadlock when the process fills the other pipe's buffer.
                    var stdoutTask = proc.StandardOutput.ReadToEndAsync();
                    var stderrTask = proc.StandardError.ReadToEndAsync();

                    if (!proc.WaitForExit(timeoutSec * 1000))
                    {
                        try { proc.Kill(); } catch { }
                        try { proc.WaitForExit(); } catch { }
                        return new CompileResult { ErrorMessage = $"Compile timed out after {timeoutSec}s" };
                    }
                    // Parameterless overload waits for redirected streams to flush.
                    proc.WaitForExit();
                    Task.WaitAll(stdoutTask, stderrTask);

                    if (proc.ExitCode != 0)
                    {
                        var stdout = stdoutTask.Result;
                        var stderr = stderrTask.Result;
                        var output = string.IsNullOrEmpty(stderr) ? stdout : stderr;
                        return new CompileResult
                        {
                            ErrorMessage = $"Compile error:\n{output}",
                            Diagnostics = CscOutputParser.Parse(stdout + "\n" + stderr),
                        };
                    }
                }

                return new CompileResult { AssemblyBytes = File.ReadAllBytes(outFile) };
            }
            catch (Exception ex)
            {
                return new CompileResult { ErrorMessage = $"Compile failed: {ex.Message}" };
            }
            finally
            {
                try { File.Delete(srcFile); } catch { }
                try { File.Delete(outFile); } catch { }
                try { File.Delete(rspFile); } catch { }
            }
        }

        private static string FindCsc(string cscOverride = null)
        {
            if (!string.IsNullOrEmpty(cscOverride))
                return cscOverride;

            var content = EditorApplication.applicationContentsPath;
            var cscDll = SearchFile(content, "csc.dll");
            if (cscDll != null) return cscDll;

            if (Application.platform == RuntimePlatform.WindowsEditor)
            {
                var cscExe = SearchFile(content, "csc.exe");
                if (cscExe != null) return cscExe;
            }

            return null;
        }

        private static string SearchFile(string dir, string name)
        {
            try
            {
                var files = Directory.GetFiles(dir, name, SearchOption.AllDirectories);
                foreach (var f in files)
                    if (Path.GetFileName(f) == name)
                        return f;
            }
            catch { }
            return null;
        }

        private static string FindDotnet(string dotnetOverride = null)
        {
            if (!string.IsNullOrEmpty(dotnetOverride))
                return dotnetOverride;

            var name = "dotnet" + (Application.platform == RuntimePlatform.WindowsEditor ? ".exe" : "");
            var found = SearchFile(EditorApplication.applicationContentsPath, name);
            if (found != null) return found;

            if (Application.platform != RuntimePlatform.WindowsEditor)
            {
                var macPaths = new[]
                {
                    "/usr/local/share/dotnet/dotnet",
                    "/opt/homebrew/bin/dotnet",
                    "/usr/local/bin/dotnet",
                };
                foreach (var p in macPaths)
                    if (File.Exists(p)) return p;
            }

            return name;
        }

        private static object Serialize(object obj, int depth)
        {
            if (obj == null) return null;
            if (depth > 4) return obj.ToString();
            var type = obj.GetType();
            if (type.IsPrimitive || type == typeof(string) || type == typeof(decimal)) return obj;
            if (type.IsEnum) return obj.ToString();
            if (type.Name.StartsWith("FixedString")) return obj.ToString();
            if (obj is IDictionary dict)
            {
                var r = new Dictionary<string, object>();
                foreach (DictionaryEntry e in dict)
                    r[e.Key.ToString()] = Serialize(e.Value, depth + 1);
                return r;
            }
            if (obj is IEnumerable enumerable)
            {
                var list = new List<object>();
                int count = 0;
                foreach (var item in enumerable)
                {
                    if (count++ >= 100) { list.Add("... (truncated at 100)"); break; }
                    list.Add(Serialize(item, depth + 1));
                }
                return list;
            }
            if (type.IsValueType || type.IsClass)
            {
                var fields = type.GetFields(BindingFlags.Public | BindingFlags.Instance);
                if (fields.Length > 0)
                {
                    var r = new Dictionary<string, object>();
                    foreach (var f in fields)
                    {
                        try { r[f.Name] = Serialize(f.GetValue(obj), depth + 1); }
                        catch { r[f.Name] = "<error>"; }
                    }
                    return r;
                }
            }
            return obj.ToString();
        }
    }
}
