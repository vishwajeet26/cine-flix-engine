import axios from "axios";

const axiosClient = axios.create({
  baseURL: "https://cine-stream-backend-hqiu.onrender.com",
  withCredentials: true,
});

export default axiosClient;
