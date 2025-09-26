import express, {
  type Request,
  type Response,
  type NextFunction,
} from "express";
import cors from "cors";
import { createServer } from "node:http";
import { Server } from "socket.io";
import nodemailer from "nodemailer";
import morgan from "morgan";
import jwt, { type JwtPayload } from "jsonwebtoken";

import { initializeApp, applicationDefault } from "firebase-admin/app";
import { getFirestore } from "firebase-admin/firestore";

import boardRouter from "./controllers/board.js";
import authRouter from "./controllers/auth.js";
import registerSocket from "./socket.js";
import { fileURLToPath } from "node:url";
import path from "node:path";

declare global {
  namespace Express {
    interface Request {
      email?: string;
    }
  }
}

export const ORDER_GAP = 1_000;

initializeApp({
  credential: applicationDefault(),
  databaseURL: process.env.FIREBASE_DB_URL!,
});

export const transporter = nodemailer.createTransport({
  service: "gmail",
  auth: {
    user: process.env.APP_EMAIL,
    pass: process.env.APP_EMAIL_PASSWORD,
  },
});

export const db = getFirestore();

const app = express();
app.use(cors());
const server = createServer(app);

const io = new Server(server, {
  cors: {
    origin: "http://localhost:5173",
  },
});

io.on("connection", (socket) => {
  registerSocket(socket);
});

app.use(express.json());
app.use(morgan("dev"));

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const distPath = path.join(__dirname, "../../ui/dist");

app.use(express.static(distPath));

export function requireAuth(
  req: Request,
  res: Response,
  next: NextFunction,
): asserts req is Request & { email: string } {
  const authHeader = req.headers.authorization;
  const token = authHeader && authHeader.split(" ")[1];

  if (!token) {
    res.status(401).json({ message: "Missing token" });
    return;
  }

  jwt.verify(token, process.env.JWT_SECRET!, (err, payload) => {
    if (err) return res.status(403).json({ message: "Invalid token" });

    req.email = (payload as JwtPayload).email;
    next();
  });
}

app.use("/api/boards", requireAuth, boardRouter);
app.use("/api/auth", authRouter);

app.use((_, res) => {
  res.sendFile(path.join(distPath, "index.html"));
});

const port = 3000;
server.listen(port, () => {
  console.log(`Server running at http://localhost:${port}`);
});
