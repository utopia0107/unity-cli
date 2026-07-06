using Newtonsoft.Json.Linq;
using NUnit.Framework;

namespace UnityCliConnector.Tests
{
    public class CommandRouterTests
    {
        static JObject Dispatch(string command, string paramsJson)
        {
            var parameters = paramsJson == null ? null : JObject.Parse(paramsJson);
            var result = CommandRouter.Dispatch(command, parameters).GetAwaiter().GetResult();
            return JObject.FromObject(result);
        }

        [Test]
        public void UnknownCommand_ReturnsUnknownCommandCode()
        {
            var resp = Dispatch("no_such_tool_xyz", "{}");
            Assert.IsFalse(resp["success"].Value<bool>());
            Assert.AreEqual("unknown_command", resp["data"]?["code"]?.ToString());
        }

        [Test]
        public void MissingRequiredParam_IsRejectedBeforeHandler()
        {
            var resp = Dispatch("cli_test_fixture", "{}");
            Assert.IsFalse(resp["success"].Value<bool>());
            Assert.AreEqual("missing_param", resp["data"]?["code"]?.ToString());
            Assert.AreEqual("target", resp["data"]?["param"]?.ToString());
        }

        [Test]
        public void PositionalArg_IsNormalizedIntoNamedParam()
        {
            var resp = Dispatch("cli_test_fixture", "{\"args\": [\"hello\"]}");
            Assert.IsTrue(resp["success"].Value<bool>(), resp.ToString());
            Assert.AreEqual("hello", resp["data"]?["target"]?.ToString());
        }

        [Test]
        public void NamedParam_WinsOverPositional()
        {
            var resp = Dispatch("cli_test_fixture", "{\"target\": \"named\", \"args\": [\"positional\"]}");
            Assert.IsTrue(resp["success"].Value<bool>(), resp.ToString());
            Assert.AreEqual("named", resp["data"]?["target"]?.ToString());
        }

        [Test]
        public void UnparseableParam_ReturnsInvalidParamCode()
        {
            var resp = Dispatch("cli_test_fixture", "{\"target\": \"x\", \"count\": \"abc\"}");
            Assert.IsFalse(resp["success"].Value<bool>());
            Assert.AreEqual("invalid_param", resp["data"]?["code"]?.ToString());
            Assert.AreEqual("count", resp["data"]?["param"]?.ToString());
        }

        [Test]
        public void NullParameters_StillValidated()
        {
            var resp = Dispatch("cli_test_fixture", null);
            Assert.IsFalse(resp["success"].Value<bool>());
            Assert.AreEqual("missing_param", resp["data"]?["code"]?.ToString());
        }
    }
}
