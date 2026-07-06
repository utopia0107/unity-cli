using System.Linq;
using Newtonsoft.Json.Linq;
using NUnit.Framework;

namespace UnityCliConnector.Tests
{
    [UnityCliTool(Name = "cli_test_fixture", Description = "Test fixture tool", Group = "test")]
    public static class DiscoveryFixtureTool
    {
        public class Parameters
        {
            [ToolParameter("Positional required value", Required = true, Position = 0)]
            public string Target { get; set; }

            [ToolParameter("renamed", "Name-overridden optional value")]
            public string OriginalName { get; set; }

            [ToolParameter("Plain int")]
            public int Count { get; set; }
        }

        public static object HandleCommand(JObject parameters)
        {
            var p = new ToolParams(parameters);
            var count = p.GetInt("count", 0);
            return new SuccessResponse("fixture ok", new { target = p.Get("target"), count });
        }
    }

    public class ToolDiscoveryTests
    {
        [Test]
        public void Find_LocatesFixtureTool()
        {
            var tool = ToolDiscovery.Find("cli_test_fixture");
            Assert.IsNotNull(tool);
            Assert.AreEqual("cli_test_fixture", tool.Name);
            Assert.AreEqual("Test fixture tool", tool.Description);
            Assert.IsNotNull(tool.Handler);
        }

        [Test]
        public void Find_UnknownReturnsNull()
        {
            Assert.IsNull(ToolDiscovery.Find("no_such_tool_xyz"));
        }

        [Test]
        public void Find_ReturnsCachedInstance()
        {
            var first = ToolDiscovery.Find("cli_test_fixture");
            var second = ToolDiscovery.Find("cli_test_fixture");
            Assert.AreSame(first, second, "repeated lookups should hit the cache");
        }

        [Test]
        public void Schema_HonorsRequiredPositionAndNameOverride()
        {
            var tool = ToolDiscovery.Find("cli_test_fixture");
            var target = tool.Schema.Single(s => s.Name == "target");
            Assert.IsTrue(target.Required);
            Assert.AreEqual(0, target.Position);

            var renamed = tool.Schema.SingleOrDefault(s => s.Name == "renamed");
            Assert.IsNotNull(renamed, "ToolParameterAttribute.Name override should win over snake_case");
            Assert.IsFalse(renamed.Required);
            Assert.AreEqual(-1, renamed.Position);

            var count = tool.Schema.Single(s => s.Name == "count");
            Assert.AreEqual("Int32", count.Type);
        }

        [Test]
        public void GetToolSchemas_IncludesBuiltinsAndFixture()
        {
            var schemas = ToolDiscovery.GetToolSchemas();
            Assert.IsTrue(schemas.Count > 5, "built-in tools should be discovered");
            var json = JArray.FromObject(schemas);
            var names = json.Select(t => t["name"]?.ToString()).ToList();
            CollectionAssert.Contains(names, "cli_test_fixture");
            CollectionAssert.Contains(names, "exec");
            CollectionAssert.Contains(names, "manage_editor");
        }
    }
}
