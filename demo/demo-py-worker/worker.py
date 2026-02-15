import time

def main(req):
    current_time = time.time()
    print(f"Worker received request at {current_time}")
    return {"message": "Hello from Cubis Workers!", "timestamp": time.strftime("%Y-%m-%dT%H:%M:%S", time.gmtime(current_time))}