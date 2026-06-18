import "./App.css";
import Home from "./components/home/Home";
import Recommended from "./components/recommended/Recommended";
import Review from "./components/review/Review";
import Header from "./components/header/Header";
import Register from "./components/register/Register";
import Login from "./components/login/Login";
import Layout from "./components/Layout";
import RequiredAuth from "./components/RequiredAuth";
import axiosClient from "./api/axiosConfig";
import useAuth from "./hooks/useAuth";
import StreamMovie from "./components/stream/StreamMovie";

import { Route, Routes, useNavigate } from "react-router-dom";

function App() {
  const navigate = useNavigate();
  const { auth, setAuth } = useAuth();

  const updateMovieReview = (imdb_id) => {
    navigate(`/review/${imdb_id}`);
  };

  const handleLogout = async () => {
    try {
      const response = await axiosClient.post("/logout", {
        user_id: auth.user_id,
      });
      console.log(response.data);
      setAuth(null);
      console.log("User logged out");
    } catch (error) {
      console.error("Error logging out:", error);
    }
  };

  return (
    <>
      <Header handleLogout={handleLogout} />

      {/* Highlight-start: Corrected structural wrapping */}
      <Routes>
        <Route path="/" element={<Layout />}>
          {/* Unprotected Public Routes */}
          <Route
            index
            element={<Home updateMovieReview={updateMovieReview} />}
          />
          <Route path="register" element={<Register />} />
          <Route path="login" element={<Login />} />

          {/* Protected Routes */}
          <Route element={<RequiredAuth />}>
            <Route path="recommended" element={<Recommended />} />
            <Route path="review/:imdb_id" element={<Review />} />
            <Route path="stream/:yt_id" element={<StreamMovie />} />
          </Route>
        </Route>
      </Routes>
      {/* Highlight-end */}
    </>
  );
}

export default App;
