import unittest
from solution import dependency_layers
class Tests(unittest.TestCase):
    def test_empty(self): self.assertEqual(dependency_layers({}),[])
    def test_layers(self): self.assertEqual(dependency_layers({'d':['b','c'],'c':['a'],'b':['a'],'a':[]}),[['a'],['b','c'],['d']])
    def test_all_ready(self): self.assertEqual(dependency_layers({'z':[],'b':[],'a':[]}),[['a','b','z']])
    def test_duplicate(self): self.assertEqual(dependency_layers({'b':['a','a'],'a':[]}),[['a'],['b']])
    def test_unknown(self):
        with self.assertRaises(ValueError): dependency_layers({'a':['missing']})
    def test_cycles(self):
        for g in ({'a':['a']},{'a':[],'b':['c'],'c':['b']}):
            with self.assertRaises(ValueError): dependency_layers(g)
    def test_generator(self): self.assertEqual(dependency_layers({'b':iter(['a','a']),'a':iter([])}),[['a'],['b']])
    def test_no_mutation(self):
        g={'b':['a'],'a':[]}; dependency_layers(g); self.assertEqual(g,{'b':['a'],'a':[]})
unittest.main()
