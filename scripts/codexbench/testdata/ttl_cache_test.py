import unittest
from solution import TTLCache
class Tests(unittest.TestCase):
    def setUp(self): self.now=0; self.c=TTLCache(2,lambda:self.now)
    def test_invalid(self):
        for n in (0,-1):
            with self.assertRaises(ValueError): TTLCache(n,lambda:0)
    def test_boundary(self):
        self.c.put('a',1,5); self.now=4.9; self.assertEqual(self.c.get('a'),1); self.now=5; self.assertEqual(self.c.get('a','gone'),'gone')
    def test_lru(self):
        self.c.put('a',1,10); self.c.put('b',2,10); self.c.get('a'); self.c.put('c',3,10); self.assertIsNone(self.c.get('b')); self.assertEqual(self.c.get('a'),1)
    def test_update(self):
        self.c.put('a',1,2); self.c.put('b',2,10); self.now=1; self.c.put('a',3,10); self.now=3; self.assertEqual(self.c.get('a'),3); self.assertEqual(self.c.get('b'),2)
    def test_expired_before_evict(self):
        self.c.put('a',1,100); self.c.put('b',2,1); self.now=2; self.c.put('c',3,10); self.assertEqual(self.c.get('a'),1); self.assertEqual(self.c.get('c'),3)
    def test_delete(self):
        self.c.put('a',1,10); self.c.put('a',2,0); self.assertEqual(self.c.get('a','x'),'x'); self.c.put('b',2,-1); self.assertIsNone(self.c.get('b'))
    def test_false_values(self):
        self.c.put('a',None,1); self.c.put('b',False,1); self.assertIsNone(self.c.get('a','bad')); self.assertIs(self.c.get('b','bad'),False)
    def test_update_order(self):
        self.c.put('a',1,10); self.c.put('b',2,10); self.c.put('a',3,10); self.c.put('c',4,10); self.assertIsNone(self.c.get('b'))
unittest.main()
