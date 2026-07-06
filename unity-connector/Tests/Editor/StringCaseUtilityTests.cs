using NUnit.Framework;

namespace UnityCliConnector.Tests
{
    public class StringCaseUtilityTests
    {
        [TestCase("ManageEditor", "manage_editor")]
        [TestCase("RefreshUnity", "refresh_unity")]
        [TestCase("RunTests", "run_tests")]
        [TestCase("ExecuteCsharp", "execute_csharp")]
        [TestCase("Exec", "exec")]
        [TestCase("HTTPServer", "http_server")]
        [TestCase("ReadHTML", "read_html")]
        [TestCase("Item2D", "item2_d")]
        [TestCase("already_snake", "already_snake")]
        [TestCase("lower", "lower")]
        [TestCase("", "")]
        public void ToSnakeCase_ConvertsAsExpected(string input, string expected)
        {
            Assert.AreEqual(expected, StringCaseUtility.ToSnakeCase(input));
        }

        [Test]
        public void ToSnakeCase_NullReturnsNull()
        {
            Assert.IsNull(StringCaseUtility.ToSnakeCase(null));
        }
    }
}
