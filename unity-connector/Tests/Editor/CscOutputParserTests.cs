using Newtonsoft.Json.Linq;
using NUnit.Framework;
using UnityCliConnector.Tools;

namespace UnityCliConnector.Tests
{
    public class CscOutputParserTests
    {
        const string SampleOutput =
            "/tmp/unity-cli-exec/ab12cd34.cs(15,9): error CS1002: ; expected\n" +
            "/tmp/unity-cli-exec/ab12cd34.cs(16,1): warning CS0219: The variable 'x' is assigned but never used\n" +
            "/tmp/unity-cli-exec/ab12cd34.cs(3,1): error CS0246: The type or namespace name 'Foo' could not be found\n" +
            "some unrelated line\n";

        [Test]
        public void Parse_ExtractsDiagnostics()
        {
            var diags = CscOutputParser.Parse(SampleOutput);
            Assert.AreEqual(3, diags.Count);

            Assert.AreEqual(15, diags[0].Line);
            Assert.AreEqual(9, diags[0].Column);
            Assert.AreEqual("error", diags[0].Severity);
            Assert.AreEqual("CS1002", diags[0].Code);
            Assert.AreEqual("; expected", diags[0].Message);

            Assert.AreEqual("warning", diags[1].Severity);
        }

        [Test]
        public void Parse_EmptyOrGarbageReturnsEmpty()
        {
            Assert.IsEmpty(CscOutputParser.Parse(null));
            Assert.IsEmpty(CscOutputParser.Parse(""));
            Assert.IsEmpty(CscOutputParser.Parse("nothing to see here"));
        }

        [Test]
        public void ToUserErrors_RemapsLinesAndDropsWarnings()
        {
            var diags = CscOutputParser.Parse(SampleOutput);
            // Header of 14 lines: user code starts at generated line 15.
            var errors = JArray.FromObject(CscOutputParser.ToUserErrors(diags, 14));

            Assert.AreEqual(2, errors.Count, "warnings should be dropped");

            Assert.AreEqual(1, errors[0]["line"].Value<int>(), "generated line 15 → user line 1");
            Assert.AreEqual("CS1002", errors[0]["code"].ToString());

            Assert.AreEqual(JTokenType.Null, errors[1]["line"].Type,
                "diagnostics in the generated header should have null line");
        }
    }
}
