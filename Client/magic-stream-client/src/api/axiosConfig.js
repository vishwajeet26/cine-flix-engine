import axios from "axios";

// Highlight-start
const BASE_URL = "https://cine-stream-backend-hqiu.onrender.com";
// Highlight-end

export default axios.create({
  baseURL: BASE_URL,
});

export const axiosPrivate = axios.create({
  baseURL: BASE_URL,
  headers: { "Content-Type": "application/json" },
  withCredentials: true, // Crucial for sending your cookies/tokens to Render!
});
