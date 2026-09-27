class TTLCache:
    def __init__(self, capacity, clock):
        raise NotImplementedError
    def put(self, key, value, ttl):
        raise NotImplementedError
    def get(self, key, default=None):
        raise NotImplementedError
