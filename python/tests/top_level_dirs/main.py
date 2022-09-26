import unittest
import sys

print(sys.path, file=sys.stderr)

class TestProtoWithPkgPath(unittest.TestCase):

    def test_cant_import_mymodule_directly(self):
        with self.assertRaises(ImportError):
            import mymodule

    def test_can_import_mymodule_by_parent_package(self):
        import mypkg.mymodule


if __name__ == '__main__':
    unittest.main()
