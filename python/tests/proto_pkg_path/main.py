import sys

import unittest
import mylib.subpackage.proto.test_pb2 as test_pb2
import mylib.subpackage.proto.common_pb2 as common_pb2

print(sys.path)

class TestProtoWithPkgPath(unittest.TestCase):

    def test_main(self):
        x = test_pb2.MyMessage(name="foo",
                               common = common_pb2.MyCommonMessage(common="common"))
        self.assertEqual(x.name, "foo")
        self.assertEqual(x.common.common, "common")

if __name__ == '__main__':
    unittest.main()
