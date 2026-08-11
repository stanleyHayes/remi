export interface ServiceTime {
  name: string;
  day: string;
  time: string;
  location: string;
}

export interface Settings {
  churchName: string;
  tagline: string;
  serviceTimes: ServiceTime[];
  socials: {
    facebook?: string;
    instagram?: string;
    youtube?: string;
    tiktok?: string;
  };
  whatsapp?: string;
  phone?: string;
  email?: string;
  address?: string;
  livestreamUrl?: string;
  isLive: boolean;
  givingCategories: string[];
  nextServiceOverride?: string | null;
}

interface ContentDoc {
  id: string;
  title: string;
  slug: string;
  contentStatus: string;
  createdAt: string;
  updatedAt: string;
}

export interface Page extends ContentDoc {
  pageKey: "history" | "mission" | "beliefs" | "visit" | "about";
  subtitle?: string;
  heroImage?: string;
  body: string;
}

export interface Leader extends ContentDoc {
  name: string;
  position: string;
  bio: string;
  photo?: string;
  order: number;
  isFounder: boolean;
}

export interface Ministry extends ContentDoc {
  name: string;
  description: string;
  image?: string;
  leaderName?: string;
  meetingTime?: string;
}

export interface Branch extends ContentDoc {
  name: string;
  address: string;
  city: string;
  phone?: string;
  email?: string;
  pastorName?: string;
  serviceTimes?: string;
  mapQuery?: string;
  image?: string;
}

export interface Sermon extends ContentDoc {
  preacher: string;
  series?: string;
  topic?: string;
  bibleRefs: string[];
  date: string;
  videoUrl?: string;
  audioUrl?: string;
  notesUrl?: string;
  image?: string;
  description?: string;
}

export interface SermonListResponse {
  items: Sermon[];
  total: number;
  page: number;
  pageSize: number;
  facets: {
    preachers: string[];
    series: string[];
    topics: string[];
  };
}

export interface EventItem extends ContentDoc {
  description?: string;
  startAt: string;
  endAt?: string;
  location?: string;
  image?: string;
  registrationEnabled: boolean;
  capacity?: number;
  registeredCount?: number;
}

export interface Announcement extends ContentDoc {
  body: string;
  publishAt: string;
  expiresAt?: string;
}

export interface Testimony extends ContentDoc {
  author: string;
  body: string;
  image?: string;
}

export interface GivingInitializeResponse {
  authorizationUrl: string;
  reference: string;
  demo?: boolean;
}
