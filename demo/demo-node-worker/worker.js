import _ from "lodash";

function main(req) {
  const apiKey = process.env.API_KEY;

  const name = _.random(1, 100);
  return { message: "Hello from Cubis Workers!", name, apiKey };
}
