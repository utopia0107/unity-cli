using Newtonsoft.Json.Linq;
using NUnit.Framework;

namespace UnityCliConnector.Tests
{
    public class ToolParamsTests
    {
        static ToolParams Params(string json) => new ToolParams(JObject.Parse(json));

        [Test]
        public void GetInt_AbsentKeyReturnsDefault()
        {
            Assert.AreEqual(1920, Params("{}").GetInt("width", 1920));
            Assert.IsNull(Params("{}").GetInt("width"));
        }

        [Test]
        public void GetInt_ParsesNumbersAndNumericStrings()
        {
            Assert.AreEqual(42, Params("{\"width\": 42}").GetInt("width"));
            Assert.AreEqual(42, Params("{\"width\": \"42\"}").GetInt("width"));
        }

        [Test]
        public void GetInt_UnparseableThrows()
        {
            var ex = Assert.Throws<ToolParamException>(() => Params("{\"width\": \"abc\"}").GetInt("width", 1920));
            Assert.AreEqual("width", ex.Param);
            StringAssert.Contains("must be an integer", ex.Message);
        }

        [Test]
        public void GetFloat_UnparseableThrows()
        {
            var ex = Assert.Throws<ToolParamException>(() => Params("{\"min\": \"fast\"}").GetFloat("min", 0f));
            Assert.AreEqual("min", ex.Param);
        }

        [Test]
        public void GetFloat_ParsesInvariantCulture()
        {
            Assert.AreEqual(0.5f, Params("{\"min\": \"0.5\"}").GetFloat("min"));
        }

        [Test]
        public void GetBool_AbsentAndEmptyReturnDefault()
        {
            Assert.IsTrue(Params("{}").GetBool("wait", true));
            Assert.IsFalse(Params("{\"wait\": \"\"}").GetBool("wait", false));
        }

        [Test]
        public void GetBool_RecognizedValuesParse()
        {
            Assert.IsTrue(Params("{\"wait\": true}").GetBool("wait"));
            Assert.IsTrue(Params("{\"wait\": \"yes\"}").GetBool("wait"));
            Assert.IsFalse(Params("{\"wait\": \"off\"}").GetBool("wait", true));
        }

        [Test]
        public void GetBool_UnrecognizedThrows()
        {
            var ex = Assert.Throws<ToolParamException>(() => Params("{\"wait\": \"maybe\"}").GetBool("wait"));
            Assert.AreEqual("wait", ex.Param);
        }

        [Test]
        public void GetRequired_EmptyStringIsMissing()
        {
            Assert.IsFalse(Params("{\"code\": \"\"}").GetRequired("code").IsSuccess);
            Assert.IsTrue(Params("{\"code\": \"return 1;\"}").GetRequired("code").IsSuccess);
        }
    }
}
