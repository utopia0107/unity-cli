using Newtonsoft.Json.Linq;
using NUnit.Framework;

namespace UnityCliConnector.Tests
{
    public class ParamCoercionTests
    {
        static JToken Token(object value) => value == null ? JValue.CreateNull() : JToken.FromObject(value);

        [Test]
        public void CoerceBool_NullTokenUsesDefault()
        {
            Assert.IsTrue(ParamCoercion.CoerceBool(null, true));
            Assert.IsFalse(ParamCoercion.CoerceBool(JValue.CreateNull(), false));
        }

        [TestCase(true, true)]
        [TestCase(false, false)]
        public void CoerceBool_BooleanToken(bool value, bool expected)
        {
            Assert.AreEqual(expected, ParamCoercion.CoerceBool(Token(value), !expected));
        }

        [TestCase("true", true)]
        [TestCase("TRUE", true)]
        [TestCase("false", false)]
        [TestCase("1", true)]
        [TestCase("yes", true)]
        [TestCase("on", true)]
        [TestCase("0", false)]
        [TestCase("no", false)]
        [TestCase("off", false)]
        public void CoerceBoolNullable_RecognizedStrings(string input, bool expected)
        {
            Assert.AreEqual(expected, ParamCoercion.CoerceBoolNullable(Token(input)));
        }

        [TestCase("garbage")]
        [TestCase("")]
        [TestCase("  ")]
        public void CoerceBoolNullable_UnrecognizedStringsReturnNull(string input)
        {
            Assert.IsNull(ParamCoercion.CoerceBoolNullable(Token(input)));
        }
    }
}
