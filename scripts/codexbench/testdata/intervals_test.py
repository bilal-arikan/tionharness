import unittest
from solution import merge_intervals
class Tests(unittest.TestCase):
    def test_empty(self): self.assertEqual(merge_intervals([]), [])
    def test_touch(self): self.assertEqual(merge_intervals([(3,5),(1,3),(8,9)]),[(1,5),(8,9)])
    def test_nested(self): self.assertEqual(merge_intervals([(1,10),(2,3),(1,10)]),[(1,10)])
    def test_negative(self): self.assertEqual(merge_intervals([(-5,-2),(-3,0),(0,0)]),[(-5,0)])
    def test_invalid(self):
        with self.assertRaises(ValueError): merge_intervals([(4,2)])
    def test_generator(self): self.assertEqual(merge_intervals(iter([(5,8),(1,2),(2,6)])),[(1,8)])
    def test_unchanged(self):
        x=[[4,6],[1,3],[3,5]]; before=[a[:] for a in x]; merge_intervals(x); self.assertEqual(x,before)
    def test_empty_intervals(self): self.assertEqual(merge_intervals([(3,3),(1,1)]),[])
unittest.main()
