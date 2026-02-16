import _ from "lodash";

function main(req) {
  const name = _.random(1, 100);
  return { message: "Hello from Cubis Workers!", name };
}
