function main(req: any): any {
  const rand = Math.random();
  console.log(`Worker received request with random number: ${rand}`);
  return { message: "Hello from Cubis Workers!", random: rand };
}
