using System.Collections.Generic;
using Newtonsoft.Json;
using Newtonsoft.Json.Linq;
using NUnit.Framework;

namespace UnityCliConnector.Tests
{
    public class JsonSettingsTests
    {
        static JObject Serialize(object obj) =>
            JObject.Parse(JsonConvert.SerializeObject(obj, JsonSettings.Settings));

        [Test]
        public void AnonymousProperties_BecomeSnakeCase()
        {
            var json = Serialize(new { firstFrame = 1, frameCount = 2, compileErrors = true });
            Assert.IsNotNull(json["first_frame"]);
            Assert.IsNotNull(json["frame_count"]);
            Assert.IsNotNull(json["compile_errors"]);
            Assert.IsNull(json["firstFrame"]);
        }

        [Test]
        public void DictionaryKeys_AreNotConverted()
        {
            var payload = new Dictionary<string, object> { ["myUserField"] = 1, ["AnotherOne"] = 2 };
            var json = Serialize(new { data = payload });
            Assert.IsNotNull(json["data"]["myUserField"], "user-code field names must pass through verbatim");
            Assert.IsNotNull(json["data"]["AnotherOne"]);
        }

        [Test]
        public void ResponseEnvelopeFields_StaySingleWordLowercase()
        {
            var json = Serialize(new SuccessResponse("hello", new { someValue = 1 }));
            Assert.IsTrue(json["success"].Value<bool>());
            Assert.AreEqual("hello", json["message"].ToString());
            Assert.IsNotNull(json["data"]["some_value"]);

            var error = Serialize(new ErrorResponse("boom"));
            Assert.IsFalse(error["success"].Value<bool>());
        }
    }
}
