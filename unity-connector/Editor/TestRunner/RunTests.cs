using System;
using System.Collections.Generic;
using System.IO;
using System.Threading.Tasks;
using Newtonsoft.Json;
using Newtonsoft.Json.Linq;
using UnityEditor.SceneManagement;
using UnityEditor.TestTools.TestRunner.Api;
using UnityEngine;
using UnityEngine.SceneManagement;
using Object = UnityEngine.Object;

namespace UnityCliConnector.TestRunner
{
    [UnityCliTool(Description = "Run Unity EditMode or PlayMode tests and return results.")]
    public static class RunTests
    {
        internal static readonly string StatusDir = Path.Combine(
            Environment.GetFolderPath(Environment.SpecialFolder.UserProfile), ".unity-cli", "status");

        public class Parameters
        {
            [ToolParameter("Test mode: EditMode or PlayMode", Required = true)]
            public string Mode { get; set; }

            [ToolParameter("Filter by namespace, class, or full test name")]
            public string Filter { get; set; }

            [ToolParameter("Run tests even when open scenes have unsaved changes")]
            public bool AllowDirtyScenes { get; set; }

            [ToolParameter("Save dirty open scenes before running tests")]
            public bool AutoSaveScenes { get; set; }

            [ToolParameter("CLI-generated test run identifier")]
            public string RunId { get; set; }

            [ToolParameter("EditMode run timeout in seconds (default 300). On expiry the request is released; the Unity test run may keep executing.")]
            public int TimeoutSec { get; set; }
        }

        private const int DefaultEditModeTimeoutSec = 300;

        public static Task<object> HandleCommand(JObject @params)
        {
            if (@params == null)
                return Task.FromResult<object>(new ErrorResponse("Parameters cannot be null."));

            var p = new ToolParams(@params);

            var modeResult = p.GetRequired("mode");
            if (!modeResult.IsSuccess)
                return Task.FromResult<object>(new ErrorResponse(modeResult.ErrorMessage));

            var modeStr = modeResult.Value.Trim();
            TestMode testMode;
            if (modeStr.Equals("EditMode", StringComparison.OrdinalIgnoreCase))
                testMode = TestMode.EditMode;
            else if (modeStr.Equals("PlayMode", StringComparison.OrdinalIgnoreCase))
                testMode = TestMode.PlayMode;
            else
                return Task.FromResult<object>(new ErrorResponse($"Unknown mode '{modeStr}'. Use EditMode or PlayMode."));

            var filter = p.Get("filter", null);
            var dirtySceneResult = PrepareDirtyScenes(
                p.GetBool("allowDirtyScenes") || p.GetBool("allow_dirty_scenes"),
                p.GetBool("autoSaveScenes") || p.GetBool("auto_save_scenes"));
            if (dirtySceneResult != null)
                return Task.FromResult<object>(dirtySceneResult);

            if (testMode == TestMode.EditMode)
            {
                var timeoutSec = p.GetInt("timeout_sec", DefaultEditModeTimeoutSec).Value;
                if (timeoutSec <= 0) timeoutSec = DefaultEditModeTimeoutSec;
                return ExecuteInProcess(testMode, filter, timeoutSec);
            }

            var runId = p.Get("runId", Guid.NewGuid().ToString("N"));
            StartPlayModeRun(filter, runId);
            return Task.FromResult<object>(new SuccessResponse("running", new { runId }));
        }

        private static Task<object> ExecuteInProcess(TestMode mode, string filter, int timeoutSec)
        {
            var tcs = new TaskCompletionSource<object>(TaskCreationOptions.RunContinuationsAsynchronously);
            var passed  = new List<string>();
            var failed  = new List<string>();
            var skipped = new List<string>();

            var api = ScriptableObject.CreateInstance<TestRunnerApi>();
            var callbacks = new TestCallbacks(
                onResult: r => CollectResult(r, passed, failed, skipped),
                onFinished: _ =>
                {
                    if (tcs.Task.IsCompleted) return;
                    Object.DestroyImmediate(api);
                    tcs.TrySetResult(BuildResponse(passed, failed, skipped));
                }
            );

            // Watchdog: a hung run must not hold the request (and the global
            // command semaphore) forever. TestRunnerApi has no reliable cancel,
            // so on expiry we release the request; the run may keep executing.
            var start = DateTime.UtcNow;
            void Watchdog()
            {
                if (tcs.Task.IsCompleted)
                {
                    UnityEditor.EditorApplication.update -= Watchdog;
                    return;
                }
                if ((DateTime.UtcNow - start).TotalSeconds < timeoutSec) return;

                UnityEditor.EditorApplication.update -= Watchdog;
                try { api.UnregisterCallbacks(callbacks); } catch { }
                Object.DestroyImmediate(api);
                tcs.TrySetResult(new ErrorResponse(
                    $"EditMode test run timed out after {timeoutSec}s. The Unity test run may still be executing.",
                    new { code = "test_timeout" }));
            }

            api.RegisterCallbacks(callbacks);
            UnityEditor.EditorApplication.update += Watchdog;
            api.Execute(new ExecutionSettings(BuildFilter(mode, filter)));
            return tcs.Task;
        }

        private static void StartPlayModeRun(string filter, string runId)
        {
            try { var f = ResultsFilePath(runId); if (File.Exists(f)) File.Delete(f); } catch { }
            TestRunnerState.MarkPending(runId, filter);

            var passed  = new List<string>();
            var failed  = new List<string>();
            var skipped = new List<string>();

            var api = ScriptableObject.CreateInstance<TestRunnerApi>();
            var callbacks = new TestCallbacks(
                onResult: r => CollectResult(r, passed, failed, skipped),
                onFinished: _ =>
                {
                    Object.DestroyImmediate(api);
                    TestRunnerState.ClearPending(runId);
                    WriteResultsFile(runId, passed, failed, skipped);
                }
            );

            api.RegisterCallbacks(callbacks);
            api.Execute(new ExecutionSettings(BuildFilter(TestMode.PlayMode, filter)));
        }

        private static object PrepareDirtyScenes(bool allowDirtyScenes, bool autoSaveScenes)
        {
            var dirtyScenes = GetDirtyOpenScenes();
            if (dirtyScenes.Count == 0)
                return null;

            if (autoSaveScenes)
            {
                foreach (var scene in dirtyScenes)
                {
                    if (string.IsNullOrEmpty(scene.path))
                        return DirtyScenesBlockedResponse(new List<string> { scene.name });
                    if (!EditorSceneManager.SaveScene(scene))
                        return DirtyScenesBlockedResponse(new List<string> { scene.path });
                }
                return null;
            }

            if (allowDirtyScenes)
                return null;

            var paths = new List<string>();
            foreach (var scene in dirtyScenes)
                paths.Add(string.IsNullOrEmpty(scene.path) ? scene.name : scene.path);

            return DirtyScenesBlockedResponse(paths);
        }

        private static List<Scene> GetDirtyOpenScenes()
        {
            var scenes = new List<Scene>();
            for (var i = 0; i < SceneManager.sceneCount; i++)
            {
                var scene = SceneManager.GetSceneAt(i);
                if (scene.isDirty)
                    scenes.Add(scene);
            }
            return scenes;
        }

        private static ErrorResponse DirtyScenesBlockedResponse(List<string> scenes)
        {
            const string message = "Open scenes have unsaved changes. Save or discard them before running tests.";
            return new ErrorResponse(message, new
            {
                blocked = "dirty-scenes",
                message,
                scenes,
            });
        }

        // --- Shared helpers (used by TestRunnerState after domain reload) ---

        internal static void CollectResult(ITestResultAdaptor result,
            List<string> passed, List<string> failed, List<string> skipped)
        {
            if (result.Test.IsSuite) return;
            var name = result.Test.FullName;
            switch (result.TestStatus)
            {
                case TestStatus.Passed:  passed.Add(name); break;
                case TestStatus.Failed:  failed.Add($"{name}: {result.Message}"); break;
                default:                 skipped.Add(name); break;
            }
        }

        internal static void WriteResultsFile(string runId, List<string> passed, List<string> failed, List<string> skipped)
        {
            var data = new
            {
                success = failed.Count == 0,
                message = failed.Count > 0
                    ? $"{failed.Count} test(s) failed."
                    : $"All {passed.Count} test(s) passed.",
                data = new
                {
                    total   = passed.Count + failed.Count + skipped.Count,
                    passed  = passed.Count,
                    failed  = failed.Count,
                    skipped = skipped.Count,
                    failures = failed,
                    passes   = passed,
                }
            };

            try
            {
                Directory.CreateDirectory(StatusDir);
                File.WriteAllText(ResultsFilePath(runId), JsonConvert.SerializeObject(data));
            }
            catch (Exception ex)
            {
                Debug.LogError($"[UnityCliConnector] Failed to write test results: {ex.Message}");
            }
        }

        internal static string ResultsFilePath(string runId) =>
            Path.Combine(StatusDir, $"test-results-{runId}.json");

        internal static object BuildResponse(List<string> passed, List<string> failed, List<string> skipped)
        {
            var summary = new
            {
                total   = passed.Count + failed.Count + skipped.Count,
                passed  = passed.Count,
                failed  = failed.Count,
                skipped = skipped.Count,
                failures = failed,
                passes   = passed,
            };
            return failed.Count > 0
                ? (object)new ErrorResponse($"{failed.Count} test(s) failed.", summary)
                : new SuccessResponse($"All {passed.Count} test(s) passed.", summary);
        }

        internal static Filter BuildFilter(TestMode mode, string filterStr)
        {
            var f = new Filter { testMode = mode };
            if (!string.IsNullOrEmpty(filterStr))
            {
                f.testNames  = new[] { filterStr };
                f.groupNames = new[] { filterStr };
            }
            return f;
        }

        internal class TestCallbacks : ICallbacks
        {
            private readonly Action<ITestResultAdaptor> _onResult;
            private readonly Action<ITestResultAdaptor> _onFinished;

            public TestCallbacks(Action<ITestResultAdaptor> onResult, Action<ITestResultAdaptor> onFinished)
            {
                _onResult   = onResult;
                _onFinished = onFinished;
            }

            public void RunStarted(ITestAdaptor testsToRun) { }
            public void RunFinished(ITestResultAdaptor result) => _onFinished(result);
            public void TestStarted(ITestAdaptor test) { }
            public void TestFinished(ITestResultAdaptor result) => _onResult(result);
        }
    }
}
