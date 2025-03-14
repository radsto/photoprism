<template>
  <div v-infinite-scroll="loadMore" class="p-page p-page-photos" style="user-select: none"
       :infinite-scroll-disabled="scrollDisabled" :infinite-scroll-distance="1200"
       :infinite-scroll-listen-for-event="'scrollRefresh'">

    <p-photo-toolbar :settings="settings" :filter="filter" :filter-change="updateQuery" :dirty="dirty"
                     :refresh="refresh"></p-photo-toolbar>

    <v-container v-if="loading" fluid class="pa-4">
      <v-progress-linear color="secondary-dark" :indeterminate="true"></v-progress-linear>
    </v-container>
    <v-container v-else fluid class="pa-0">
      <p-scroll-top></p-scroll-top>

      <p-photo-clipboard :refresh="refresh" :selection="selection" :context="context"></p-photo-clipboard>

      <p-photo-mosaic v-if="settings.view === 'mosaic'"
                      :context="context"
                      :photos="results"
                      :select-mode="selectMode"
                      :filter="filter"
                      :edit-photo="editPhoto"
                      :open-photo="openPhoto"></p-photo-mosaic>
      <p-photo-list v-else-if="settings.view === 'list'"
                    :context="context"
                    :photos="results"
                    :select-mode="selectMode"
                    :filter="filter"
                    :open-photo="openPhoto"
                    :edit-photo="editPhoto"
                    :open-location="openLocation"></p-photo-list>
      <p-photo-cards v-else
                     :context="context"
                     :photos="results"
                     :select-mode="selectMode"
                     :filter="filter"
                     :open-photo="openPhoto"
                     :edit-photo="editPhoto"
                     :open-location="openLocation"></p-photo-cards>
    </v-container>
  </div>
</template>

<script>
import {Photo, TypeLive, TypeRaw, TypeVideo} from "model/photo";
import Thumb from "model/thumb";
import Event from "pubsub-js";

export default {
  name: 'PPagePhotos',
  props: {
    staticFilter: Object
  },
  data() {
    const query = this.$route.query;
    const routeName = this.$route.name;
    const order = this.sortOrder();
    const camera = query['camera'] ? parseInt(query['camera']) : 0;
    const q = query['q'] ? query['q'] : '';
    const country = query['country'] ? query['country'] : '';
    const lens = query['lens'] ? parseInt(query['lens']) : 0;
    const year = query['year'] ? parseInt(query['year']) : 0;
    const month = query['month'] ? parseInt(query['month']) : 0;
    const color = query['color'] ? query['color'] : '';
    const label = query['label'] ? query['label'] : '';
    const view = this.viewType();
    const filter = {
      country: country,
      camera: camera,
      lens: lens,
      label: label,
      year: year,
      month: month,
      color: color,
      order: order,
      q: q,
    };

    const settings = this.$config.settings();

    if (settings && settings.features.private) {
      filter.public = true;
    }

    if (settings && settings.features.review && (!this.staticFilter || !("quality" in this.staticFilter))) {
      filter.quality = 3;
    }

    return {
      subscriptions: [],
      listen: false,
      dirty: false,
      complete: false,
      results: [],
      scrollDisabled: true,
      batchSize: Photo.batchSize(),
      offset: 0,
      page: 0,
      selection: this.$clipboard.selection,
      settings: {view: view},
      filter: filter,
      lastFilter: {},
      routeName: routeName,
      loading: true,
      viewer: {
        results: [],
        loading: false,
      },
    };
  },
  computed: {
    selectMode: function() {
      return this.selection.length > 0;
    },
    context: function () {
      if (!this.staticFilter) {
        return "photos";
      }

      if (this.staticFilter.review) {
        return "review";
      } else if (this.staticFilter.archived) {
        return "archive";
      } else if (this.staticFilter.favorite) {
        return "favorites";
      }

      return "";
    }
  },
  watch: {
    '$route'() {
      const query = this.$route.query;

      this.filter.q = query['q'] ? query['q'] : '';
      this.filter.camera = query['camera'] ? parseInt(query['camera']) : 0;
      this.filter.country = query['country'] ? query['country'] : '';
      this.filter.lens = query['lens'] ? parseInt(query['lens']) : 0;
      this.filter.year = query['year'] ? parseInt(query['year']) : 0;
      this.filter.month = query['month'] ? parseInt(query['month']) : 0;
      this.filter.color = query['color'] ? query['color'] : '';
      this.filter.label = query['label'] ? query['label'] : '';
      this.filter.order = this.sortOrder();
      this.settings.view = this.viewType();
      this.lastFilter = {};
      this.routeName = this.$route.name;
      this.search();
    }
  },
  created() {
    this.search();

    this.subscriptions.push(Event.subscribe("import.completed", (ev, data) => this.onImportCompleted(ev, data)));
    this.subscriptions.push(Event.subscribe("photos", (ev, data) => this.onUpdate(ev, data)));

    this.subscriptions.push(Event.subscribe("touchmove.top", () => this.refresh()));
    this.subscriptions.push(Event.subscribe("touchmove.bottom", () => this.loadMore()));
  },
  destroyed() {
    for (let i = 0; i < this.subscriptions.length; i++) {
      Event.unsubscribe(this.subscriptions[i]);
    }
  },
  methods: {
    viewType() {
      let queryParam = this.$route.query["view"];
      let storedType = window.localStorage.getItem("photo_view");

      if (queryParam) {
        window.localStorage.setItem("photo_view", queryParam);
        return queryParam;
      } else if (storedType) {
        return storedType;
      } else if (window.innerWidth < 960) {
        return 'mosaic';
      }

      return 'cards';
    },
    sortOrder() {
      let queryParam = this.$route.query["order"];
      let storedType = window.localStorage.getItem("photo_order");

      if (queryParam) {
        window.localStorage.setItem("photo_order", queryParam);
        return queryParam;
      } else if (storedType) {
        return storedType;
      }

      return "newest";
    },
    openLocation(index) {
      const photo = this.results[index];

      if (photo.CellID && photo.CellID !== "zz") {
        this.$router.push({name: "place", params: {q: photo.CellID}});
      } else if (photo.PlaceID && photo.PlaceID !== "zz") {
        this.$router.push({name: "place", params: {q: photo.PlaceID}});
      } else if (photo.Country && photo.Country !== "zz") {
        this.$router.push({name: "place", params: {q: "country:" + photo.Country}});
      } else {
        this.$notify.warn("unknown location");
      }
    },
    editPhoto(index) {
      let selection = this.results.map((p) => {
        return p.getId();
      });

      // Open Edit Dialog
      Event.publish("dialog.edit", {selection: selection, album: null, index: index});
    },
    openPhoto(index, showMerged) {
      if (this.loading || this.viewer.loading || !this.results[index]) {
        return false;
      }

      const selected = this.results[index];

      // Don't open as stack when user is selecting pictures, or a RAW has only one JPEG.
      if (this.selection.length > 0 || selected.Type === TypeRaw && selected.jpegFiles().length < 2) {
        showMerged = false;
      }

      if (showMerged && selected.Type === TypeLive || selected.Type === TypeVideo) {
        if (selected.isPlayable()) {
          this.$viewer.play({video: selected});
        } else {
          this.$viewer.show(Thumb.fromPhotos(this.results), index);
        }
      } else if (showMerged) {
        this.$viewer.show(Thumb.fromFiles([selected]), 0);
      } else {
        this.viewerResults().then((results) => {
          const thumbsIndex = results.findIndex(result => result.UID === selected.UID);

          if (thumbsIndex < 0) {
            this.$viewer.show(Thumb.fromPhotos(this.results), index);
          } else {
            this.$viewer.show(Thumb.fromPhotos(results), thumbsIndex);
          }
        });
      }
    },
    viewerResults() {
      if (this.complete || this.loading || this.viewer.loading) {
        return Promise.resolve(this.results);
      }

      if (this.viewer.results.length > (this.results.length + this.batchSize)) {
        return Promise.resolve(this.viewer.results);
      }

      this.viewer.loading = true;

      const params = {
        count: this.batchSize * (this.page + 6),
        offset: 0,
        merged: true,
      };

      Object.assign(params, this.lastFilter);

      if (this.staticFilter) {
        Object.assign(params, this.staticFilter);
      }

      return Photo.search(params).then((resp) => {
        // Success.
        this.viewer.loading = false;
        this.viewer.results = resp.models;
        return Promise.resolve(this.viewer.results);
      }, () => {
        // Error.
        this.viewer.loading = false;
        this.viewer.results = [];
        return Promise.resolve(this.results);
      }
      );
    },
    loadMore() {
      if (this.scrollDisabled) return;

      this.scrollDisabled = true;
      this.listen = false;

      const count = this.dirty ? (this.page + 2) * this.batchSize : this.batchSize;
      const offset = this.dirty ? 0 : this.offset;

      const params = {
        count: count,
        offset: offset,
        merged: true,
      };

      Object.assign(params, this.lastFilter);

      if (this.staticFilter) {
        Object.assign(params, this.staticFilter);
      }

      Photo.search(params).then(response => {
        this.results = Photo.mergeResponse(this.results, response);
        this.complete = (response.count < count);
        this.scrollDisabled = this.complete;

        if (this.complete) {
          this.offset = offset;

          if (this.results.length > 1) {
            this.$notify.info(this.$gettextInterpolate(this.$gettext("%{n} pictures found"), {n: this.results.length}));
          }
        } else if (this.results.length >= Photo.limit()) {
          this.offset = offset;
          this.complete = true;
          this.scrollDisabled = true;
          this.$notify.warn(this.$gettext("Can't load more, limit reached"));
        } else {
          this.offset = offset + count;
          this.page++;

          this.$nextTick(() => {
            if (this.$root.$el.clientHeight <= window.document.documentElement.clientHeight + 300) {
              this.$emit("scrollRefresh");
            }
          });
        }
      }).catch(() => {
        this.scrollDisabled = false;
      }).finally(() => {
        this.dirty = false;
        this.loading = false;
        this.listen = true;

        if (offset === 0) {
          this.viewerResults();
        }
      });
    },
    updateQuery() {
      this.filter.q = this.filter.q.trim();

      const query = {
        view: this.settings.view
      };

      Object.assign(query, this.filter);

      for (let key in query) {
        if (query[key] === undefined || !query[key]) {
          delete query[key];
        }
      }

      if (JSON.stringify(this.$route.query) === JSON.stringify(query)) {
        return;
      }

      this.$router.replace({query});
    },
    searchParams() {
      const params = {
        count: this.batchSize,
        offset: this.offset,
        merged: true,
      };

      Object.assign(params, this.filter);

      if (this.staticFilter) {
        Object.assign(params, this.staticFilter);
      }

      return params;
    },
    refresh() {
      if (this.loading) {
        return;
      }

      this.loading = true;
      this.page = 0;
      this.dirty = true;
      this.complete = false;
      this.scrollDisabled = false;

      this.loadMore();
    },
    search() {
      this.scrollDisabled = true;

      // Don't query the same data more than once
      if (JSON.stringify(this.lastFilter) === JSON.stringify(this.filter)) {
        this.$nextTick(() => this.$emit("scrollRefresh"));
        return;
      }

      Object.assign(this.lastFilter, this.filter);

      this.offset = 0;
      this.page = 0;
      this.loading = true;
      this.listen = false;
      this.complete = false;

      const params = this.searchParams();

      Photo.search(params).then(response => {
        this.offset = this.batchSize;
        this.results = response.models;
        this.viewer.results = [];
        this.complete = (response.count < this.batchSize);
        this.scrollDisabled = this.complete;

        if (this.complete) {
          if (!this.results.length) {
            this.$notify.warn(this.$gettext("No pictures found"));
          } else if (this.results.length === 1) {
            this.$notify.info(this.$gettext("One picture found"));
          } else {
            this.$notify.info(this.$gettextInterpolate(this.$gettext("%{n} pictures found"), {n: this.results.length}));
          }
        } else {
          this.$notify.info(this.$gettextInterpolate(this.$gettext("More than %{n} pictures found"), {n: 50}));

          this.$nextTick(() => {
            if (this.$root.$el.clientHeight <= window.document.documentElement.clientHeight + 300) {
              this.$emit("scrollRefresh");
            }
          });
        }
      }).finally(() => {
        this.dirty = false;
        this.loading = false;
        this.listen = true;

        this.viewerResults();
      });
    },
    onImportCompleted() {
      if (!this.listen) return;

      this.loadMore();
    },
    updateResults(entity) {
      this.results.filter((m) => m.UID === entity.UID).forEach((m) => {
        for (let key in entity) {
          if (key !== "UID" && entity.hasOwnProperty(key) && entity[key] != null && typeof entity[key] !== "object") {
            m[key] = entity[key];
          }
        }
      });

      this.viewer.results.filter((m) => m.UID === entity.UID).forEach((m) => {
        for (let key in entity) {
          if (key !== "UID" && entity.hasOwnProperty(key) && entity[key] != null && typeof entity[key] !== "object") {
            m[key] = entity[key];
          }
        }
      });
    },
    removeResult(results, uid) {
      const index = results.findIndex((m) => m.UID === uid);

      if (index >= 0) {
        results.splice(index, 1);
      }
    },
    onUpdate(ev, data) {
      if (!this.listen) return;

      if (!data || !data.entities || !Array.isArray(data.entities)) {
        return;
      }

      const type = ev.split('.')[1];

      switch (type) {
        case 'updated':
          for (let i = 0; i < data.entities.length; i++) {
            const values = data.entities[i];

            if (this.context === "review" && values.Quality >= 3) {
              this.removeResult(this.results, values.UID);
              this.removeResult(this.viewer.results, values.UID);
              this.$clipboard.removeId(values.UID);
            } else {
              this.updateResults(values);
            }
          }
          break;
        case 'restored':
          this.dirty = true;
          this.complete = false;

          if (this.context !== "archive") break;

          for (let i = 0; i < data.entities.length; i++) {
            const uid = data.entities[i];

            this.removeResult(this.results, uid);
            this.removeResult(this.viewer.results, uid);
          }

          break;
        case 'archived':
          this.dirty = true;
          this.complete = false;

          if (this.context === "archive") break;

          for (let i = 0; i < data.entities.length; i++) {
            const uid = data.entities[i];

            this.removeResult(this.results, uid);
            this.removeResult(this.viewer.results, uid);
            this.$clipboard.removeId(uid);
          }

          break;
        case 'deleted':
          this.dirty = true;
          this.complete = false;

          for (let i = 0; i < data.entities.length; i++) {
            const uid = data.entities[i];

            this.removeResult(this.results, uid);
            this.removeResult(this.viewer.results, uid);
            this.$clipboard.removeId(uid);
          }

          break;
        case 'created':
          this.dirty = true;
          this.scrollDisabled = false;
          this.complete = false;

          break;
        default:
          console.warn("unexpected event type", ev);
      }

      // TODO: Needed?
      this.$forceUpdate();
    },
  },
};
</script>
