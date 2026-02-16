import time
import numpy as np

def main(req):
    current_time = time.time()
    print(f"Worker received request at {current_time}")
    computation_result = np.random.rand(1000, 1000).sum()  # Simulate some computation
    return {"message": "Hello from Cubis Workers!", "timestamp": time.strftime("%Y-%m-%dT%H:%M:%S", time.gmtime(current_time)), "computation_result": computation_result}